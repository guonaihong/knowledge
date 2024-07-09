### 分析

* Put: 本地P对应的private, 如果有值，就保存到队列里面
* Get: 本地P对应的private, 然后是队列里面找，找不到就偷取别的P的值，再找不到就找gc之前的数据sync.Pool的值

### 代码注释版本

```go
// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sync

import (
 "internal/race"
 "runtime"
 "sync/atomic"
 "unsafe"
)

// A Pool 是一个临时对象的集合，可以单独保存和检索。
// 存储在 Pool 中的任何项目都可能随时自动删除，而无需通知。
// 如果删除时 Pool 持有该项目的唯一引用，则该项目可能会被释放。
// Pool 可以安全地被多个 goroutine 同时使用。
// Pool 的目的是缓存已分配但未使用的项目以供以后重用，从而减轻垃圾收集器的压力。
// 也就是说，它使得构建高效的线程安全自由列表变得容易。但是，它不适用于所有自由列表。
// 一个合适的 Pool 使用场景是管理一组临时项目，这些项目在并发独立的客户端之间静默共享并可能重用。
// Pool 提供了一种方法，可以在许多客户端之间分摊分配开销。
// 一个很好的 Pool 使用示例是在 fmt 包中，它维护了一个动态大小的临时输出缓冲区存储。
// 该存储在负载下（当许多 goroutine 正在积极打印时）扩展，并在静止时缩小。
// 另一方面，作为短期对象的一部分维护的自由列表不适合使用 Pool，因为在这种情况下开销不会很好地分摊。
// 对于这种情况，让此类对象实现自己的自由列表更有效。
// Pool 在使用后不得复制。
// 在 Go 内存模型术语中，调用 Put(x) “同步于” 调用 Get 返回相同的值 x。
// 类似地，调用 New 返回 x “同步于” 调用 Get 返回相同的值 x。
type Pool struct {
 noCopy noCopy

 local     unsafe.Pointer // 每个 P 的本地固定大小池，实际类型是 [P]poolLocal
 localSize uintptr        // 本地数组的大小

 victim     unsafe.Pointer // 上一个周期的本地池
 victimSize uintptr        // 受害者数组的大小

 // New 可选地指定一个函数，当 Get 将返回 nil 时生成一个值。
 // 在调用 Get 期间不能更改它。
 New func() any
}

// 本地每个 P 的 Pool 附录。
type poolLocalInternal struct {
 private any       // 只能由各自的 P 使用。
 shared  poolChain // 本地 P 可以 pushHead/popHead；任何 P 都可以 popTail。
}

type poolLocal struct {
 poolLocalInternal

 // 防止在广泛的平台上的错误共享，其中 128 mod (缓存行大小) = 0。
 pad [128 - unsafe.Sizeof(poolLocalInternal{})%128]byte
}

// 来自 runtime
//go:linkname runtime_randn runtime.randn
func runtime_randn(n uint32) uint32

var poolRaceHash [128]uint64

// poolRaceAddr 返回用于竞争检测逻辑的同步点地址。
// 我们不直接使用 x 中存储的实际指针，以免与其他同步地址冲突。
// 相反，我们通过散列指针来获取 poolRaceHash 中的索引。
// 参见 golang.org/cl/31589 的讨论。
func poolRaceAddr(x any) unsafe.Pointer {
 ptr := uintptr((*[2]unsafe.Pointer)(unsafe.Pointer(&x))[1])
 h := uint32((uint64(uint32(ptr)) * 0x85ebca6b) >> 16)
 return unsafe.Pointer(&poolRaceHash[h%uint32(len(poolRaceHash))])
}

// Put 将 x 添加到池中。
func (p *Pool) Put(x any) {
 if x == nil {
  return
 }
 if race.Enabled {
  if runtime_randn(4) == 0 {
   // 随机丢弃 x。
   return
  }
  race.ReleaseMerge(poolRaceAddr(x))
  race.Disable()
 }
 l, _ := p.pin()
 if l.private == nil {
  l.private = x
 } else {
  l.shared.pushHead(x)
 }
 runtime_procUnpin()
 if race.Enabled {
  race.Enable()
 }
}

// Get 从 Pool 中选择任意项目，将其从 Pool 中移除，并将其返回给调用者。
// Get 可能会选择忽略池并将其视为空。
// 调用者不应假设传递给 Put 的值与 Get 返回的值之间存在任何关系。
// 如果 Get 将返回 nil 且 p.New 非 nil，则 Get 返回调用 p.New 的结果。
func (p *Pool) Get() any {
 if race.Enabled {
  race.Disable()
 }
 l, pid := p.pin()
 x := l.private
 l.private = nil
 if x == nil {
  // 尝试弹出本地分片的头部。我们更喜欢头部而不是尾部以重用的时间局部性。
  x, _ = l.shared.popHead()
  if x == nil {
   x = p.getSlow(pid)
  }
 }
 runtime_procUnpin()
 if race.Enabled {
  race.Enable()
  if x != nil {
   race.Acquire(poolRaceAddr(x))
  }
 }
 if x == nil && p.New != nil {
  x = p.New()
 }
 return x
}

func (p *Pool) getSlow(pid int) any {
 // 参见 pin 中的注释关于加载顺序。
 size := runtime_LoadAcquintptr(&p.localSize) // 加载-获取
 locals := p.local                            // 加载-消费
 // 尝试从其他处理器偷取一个元素。
 for i := 0; i < int(size); i++ {
  l := indexLocal(locals, (pid+i+1)%int(size))
  if x, _ := l.shared.popTail(); x != nil {
   return x
  }
 }

 // 尝试受害者缓存。我们在尝试从所有主缓存偷取后执行此操作，因为我们希望受害者缓存中的对象尽可能老化。
 size = atomic.LoadUintptr(&p.victimSize)
 if uintptr(pid) >= size {
  return nil
 }
 locals = p.victim
 l := indexLocal(locals, pid)
 if x := l.private; x != nil {
  l.private = nil
  return x
 }
 for i := 0; i < int(size); i++ {
  l := indexLocal(locals, (pid+i)%int(size))
  if x, _ := l.shared.popTail(); x != nil {
   return x
  }
 }

 // 将受害者缓存标记为空，以便未来的获取不会打扰它。
 atomic.StoreUintptr(&p.victimSize, 0)

 return nil
}

// pin 将当前 goroutine 固定到 P，禁用抢占，并返回 P 的 poolLocal 池和 P 的 ID。
// 调用者必须在完成池后调用 runtime_procUnpin()。
func (p *Pool) pin() (*poolLocal, int) {
 // 检查 p 是否为 nil 以获取恐慌。
 // 否则，在 m 被固定时发生 nil 解引用，导致致命错误而不是恐慌。
 if p == nil {
  panic("nil Pool")
 }

 pid := runtime_procPin()
 // 在 pinSlow 中我们存储到 local 然后到 localSize，这里我们以相反的顺序加载。
 // 由于我们已禁用抢占，GC 不能在两者之间发生。
 // 因此，这里我们必须观察到 local 至少与 localSize 一样大。
 // 我们可以观察到更新的/更大的 local，这是可以的（我们必须观察到它的零初始化）。
 s := runtime_LoadAcquintptr(&p.localSize) // 加载-获取
 l := p.local                              // 加载-消费
 if uintptr(pid) < s {
  return indexLocal(l, pid), pid
 }
 return p.pinSlow()
}

func (p *Pool) pinSlow() (*poolLocal, int) {
 // 在互斥锁下重试。
 // 不能在固定时锁定互斥锁。
 runtime_procUnpin()
 allPoolsMu.Lock()
 defer allPoolsMu.Unlock()
 pid := runtime_procPin()
 // poolCleanup 不会在我们被固定时被调用。
 s := p.localSize
 l := p.local
 if uintptr(pid) < s {
  return indexLocal(l, pid), pid
 }
 if p.local == nil {
  allPools = append(allPools, p)
 }
 // 如果 GOMAXPROCS 在两次 GC 之间发生变化，我们将重新分配数组并丢失旧数组。
 size := runtime.GOMAXPROCS(0)
 local := make([]poolLocal, size)
 atomic.StorePointer(&p.local, unsafe.Pointer(&local[0])) // 存储-释放
 runtime_StoreReluintptr(&p.localSize, uintptr(size))     // 存储-释放
 return &local[pid], pid
}

func poolCleanup() {
 // 此函数在世界停止时被调用，在垃圾收集开始时。
 // 它不得分配，并且可能不应该调用任何运行时函数。

 // 因为世界已停止，所以没有池用户可以处于固定部分（实际上，这使得所有 P 都被固定）。

 // 从所有池中删除受害者缓存。
 for _, p := range oldPools {
  p.victim = nil
  p.victimSize = 0
 }

 // 将主缓存移动到受害者缓存。
 for _, p := range allPools {
  p.victim = p.local
  p.victimSize = p.localSize
  p.local = nil
  p.localSize = 0
 }

 // 具有非空主缓存的池现在具有非空受害者缓存，并且没有池具有主缓存。
 oldPools, allPools = allPools, nil
}

var (
 allPoolsMu Mutex

 // allPools 是具有非空主缓存的池的集合。
 // 受保护于 1) allPoolsMu 和固定 或 2) STW。
 allPools []*Pool

 // oldPools 是可能具有非空受害者缓存的池的集合。
 // 受保护于 STW。
 oldPools []*Pool
)

func init() {
 runtime_registerPoolCleanup(poolCleanup)
}

func indexLocal(l unsafe.Pointer, i int) *poolLocal {
 lp := unsafe.Pointer(uintptr(l) + uintptr(i)*unsafe.Sizeof(poolLocal{}))
 return (*poolLocal)(lp)
}

// 在 runtime 中实现。
func runtime_registerPoolCleanup(cleanup func())
func runtime_procPin() int
func runtime_procUnpin()

// 以下在 runtime/internal/atomic 中实现，
// 编译器也知道将符号链接名称内置到此包中。

//go:linkname runtime_LoadAcquintptr runtime/internal/atomic.LoadAcquintptr
func runtime_LoadAcquintptr(ptr *uintptr) uintptr

//go:linkname runtime_StoreReluintptr runtime/internal/atomic.StoreReluintptr
func runtime_StoreReluintptr(ptr *uintptr, val uintptr) uintptr
```
