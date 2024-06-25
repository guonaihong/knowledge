### 加注释代码

```go
// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sync

import (
 "sync/atomic"
 "unsafe"
)

// poolDequeue 是一个无锁的固定大小单生产者、多消费者的队列。
// 单个生产者可以从头部同时进行推送和弹出操作，而消费者可以从尾部弹出。
// 它具有一个附加特性，即它会清空未使用的槽位以避免不必要的对象保留。
// 这对于 sync.Pool 很重要，但通常不是文献中考虑的属性。
type poolDequeue struct {
 // headTail 将一个 32 位的头部索引和一个 32 位的尾部索引打包在一起。
 // 两者都是 vals 模 len(vals)-1 的索引。
 // tail = 队列中最旧数据的位置
 // head = 下一个要填充的槽位位置
 // 范围 [tail, head) 中的槽位由消费者拥有。
 // 消费者在清空槽位之前继续拥有该槽位，此时所有权传递给生产者。
 // 头部索引存储在最高有效位中，以便我们可以原子地增加它，并且溢出是无害的。
 headTail atomic.Uint64

 // vals 是存储在此 dequeue 中的 interface{} 值的环形缓冲区。
 // 其大小必须是 2 的幂。
 // vals[i].typ 为 nil 表示槽位为空，否则为非 nil。
 // 直到尾部索引移动到该槽位之后并且 typ 被设置为 nil，槽位仍然在使用中。
 // 消费者原子地将此设置为 nil，生产者原子地读取。
 vals []eface
}

type eface struct {
 typ, val unsafe.Pointer
}

const dequeueBits = 32

// dequeueLimit 是 poolDequeue 的最大大小。
// 这必须不大于 (1<<dequeueBits)/2，因为检测满状态依赖于环形缓冲区环绕而不依赖于索引环绕。
// 我们除以 4 以便在 32 位系统上适合 int。
const dequeueLimit = (1 << dequeueBits) / 4

// dequeueNil 在 poolDequeue 中用于表示 interface{}(nil)。
// 由于我们使用 nil 来表示空槽位，因此我们需要一个哨兵值来表示 nil。
type dequeueNil *struct{}

func (d *poolDequeue) unpack(ptrs uint64) (head, tail uint32) {
 const mask = 1<<dequeueBits - 1
 head = uint32((ptrs >> dequeueBits) & mask)
 tail = uint32(ptrs & mask)
 return
}

func (d *poolDequeue) pack(head, tail uint32) uint64 {
 const mask = 1<<dequeueBits - 1
 return (uint64(head) << dequeueBits) |
  uint64(tail&mask)
}

// pushHead 在队列头部添加 val。如果队列已满，则返回 false。它只能由单个生产者调用。
func (d *poolDequeue) pushHead(val any) bool {
 ptrs := d.headTail.Load()
 head, tail := d.unpack(ptrs)
 if (tail+uint32(len(d.vals)))&(1<<dequeueBits-1) == head {
  // 队列已满。
  return false
 }
 slot := &d.vals[head&uint32(len(d.vals)-1)]

 // 检查头部槽位是否已被 popTail 释放。
 typ := atomic.LoadPointer(&slot.typ)
 if typ != nil {
  // 另一个 goroutine 仍在清理尾部，所以队列实际上仍然已满。
  return false
 }

 // 头部槽位空闲，所以我们拥有它。
 if val == nil {
  val = dequeueNil(nil)
 }
 *(*any)(unsafe.Pointer(slot)) = val

 // 增加头部。这会将槽位的所有权传递给 popTail，并作为写入槽位的存储屏障。
 d.headTail.Add(1 << dequeueBits)
 return true
}

// popHead 从队列头部移除并返回元素。如果队列为空，则返回 false。它只能由单个生产者调用。
func (d *poolDequeue) popHead() (any, bool) {
 var slot *eface
 for {
  ptrs := d.headTail.Load()
  head, tail := d.unpack(ptrs)
  if tail == head {
   // 队列为空。
   return nil, false
  }

  // 确认尾部并减少头部。我们在读取值之前执行此操作，以收回此槽位的所有权。
  head--
  ptrs2 := d.pack(head, tail)
  if d.headTail.CompareAndSwap(ptrs, ptrs2) {
   // 我们成功收回了槽位。
   slot = &d.vals[head&uint32(len(d.vals)-1)]
   break
  }
 }

 val := *(*any)(unsafe.Pointer(slot))
 if val == dequeueNil(nil) {
  val = nil
 }
 // 清空槽位。与 popTail 不同，这不会与 pushHead 竞争，因此我们不需要在这里小心。
 *slot = eface{}
 return val, true
}

// popTail 从队列尾部移除并返回元素。如果队列为空，则返回 false。它可以由任意数量的消费者调用。
func (d *poolDequeue) popTail() (any, bool) {
 var slot *eface
 for {
  ptrs := d.headTail.Load()
  head, tail := d.unpack(ptrs)
  if tail == head {
   // 队列为空。
   return nil, false
  }

  // 确认头部和尾部（对于我们上面的推测检查）并增加尾部。如果成功，则我们拥有尾部槽位。
  ptrs2 := d.pack(head, tail+1)
  if d.headTail.CompareAndSwap(ptrs, ptrs2) {
   // 成功。
   slot = &d.vals[tail&uint32(len(d.vals)-1)]
   break
  }
 }

 // 我们现在拥有槽位。
 val := *(*any)(unsafe.Pointer(slot))
 if val == dequeueNil(nil) {
  val = nil
 }

 // 告诉 pushHead 我们已经完成了这个槽位。清空槽位也很重要，这样我们就不会留下可能使对象比必要时间更长存活的引用。
 // 我们首先写入 val，然后通过原子地写入 typ 来发布我们已完成此槽位的消息。
 slot.val = nil
 atomic.StorePointer(&slot.typ, nil)
 // 此时 pushHead 拥有槽位。

 return val, true
}

// poolChain 是 poolDequeue 的动态大小版本。
// 这实现为一个双向链表队列，其中每个 dequeue 的大小是前一个的两倍。一旦一个 dequeue 填满，就会分配一个新的，并且只向最新的 dequeue 推送。
// 弹出操作发生在列表的另一端，一旦一个 dequeue 被耗尽，它就会从列表中移除。
type poolChain struct {
 // head 是推送到的 poolDequeue。这只被生产者访问，因此不需要同步。
 head *poolChainElt

 // tail 是 popTail 的 poolDequeue。这被消费者访问，因此读取和写入必须是原子的。
 tail *poolChainElt
}

type poolChainElt struct {
 poolDequeue

 // next 和 prev 链接到此 poolChain 中相邻的 poolChainElts。
 // next 由生产者原子地写入，由消费者原子地读取。它只从 nil 过渡到非 nil。
 // prev 由消费者原子地写入，由生产者原子地读取。它只从非 nil 过渡到 nil。
 next, prev *poolChainElt
}

func storePoolChainElt(pp **poolChainElt, v *poolChainElt) {
 atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(pp)), unsafe.Pointer(v))
}

func loadPoolChainElt(pp **poolChainElt) *poolChainElt {
 return (*poolChainElt)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(pp))))
}

func (c *poolChain) pushHead(val any) {
 d := c.head
 if d == nil {
  // 初始化链表。
  const initSize = 8 // 必须是 2 的幂
  d = new(poolChainElt)
  d.vals = make([]eface, initSize)
  c.head = d
  storePoolChainElt(&c.tail, d)
 }

 if d.pushHead(val) {
  return
 }

 // 当前 dequeue 已满。分配一个大小为两倍的新 dequeue。
 newSize := len(d.vals) * 2
 if newSize >= dequeueLimit {
  // 不能再大了。
  newSize = dequeueLimit
 }

 d2 := &poolChainElt{prev: d}
 d2.vals = make([]eface, newSize)
 c.head = d2
 storePoolChainElt(&d.next, d2)
 d2.pushHead(val)
}

func (c *poolChain) popHead() (any, bool) {
 d := c.head
 for d != nil {
  if val, ok := d.popHead(); ok {
   return val, ok
  }
  // 前一个 dequeue 中可能仍有未消费的元素，因此尝试回退。
  d = loadPoolChainElt(&d.prev)
 }
 return nil, false
}

func (c *poolChain) popTail() (any, bool) {
 d := loadPoolChainElt(&c.tail)
 if d == nil {
  return nil, false
 }

 for {
  // 重要的是我们在弹出尾部之前加载下一个指针。
  // 通常，d 可能暂时为空，但如果 pop 之前 next 非 nil 且 pop 失败，则 d 永久为空，
  // 这是安全地从链表中移除 d 的唯一条件。
  d2 := loadPoolChainElt(&d.next)

  if val, ok := d.popTail(); ok {
   return val, ok
  }

  if d2 == nil {
   // 这是唯一的 dequeue。它现在为空，但将来可能会被推送。
   return nil, false
  }

  // 链表的尾部已被耗尽，因此移动到下一个 dequeue。
  // 尝试从链表中移除它，以便下一个 pop 不必再次查看空 dequeue。
  if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&c.tail)), unsafe.Pointer(d), unsafe.Pointer(d2)) {
   // 我们赢得了竞争。清除 prev 指针，以便垃圾收集器可以收集空的 dequeue，
   // 并且 popHead 不会回退得太远。
   storePoolChainElt(&d2.prev, nil)
  }
  d = d2
 }
}
```
