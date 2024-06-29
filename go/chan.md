### 说说什么是channel ?

channel 的实现代码位于 <https://go.dev/src/runtime/chan.go>
channel 本质上是带阻塞效果的循环队列, 如果声明带缓存的chan, sendx 记录写入的位置，recvx 记录读取的位置

### 一、write流程

先处理几种特殊情况

1. 没有初始化的chan, 直接卡住

* 使用代码

```go
var c chan bool
c <- true

```

* 实现代码

```go
 if c == nil {
  if !block { // 非阻塞的
   return false
  }
  gopark(nil, nil, waitReasonChanSendNilChan, traceBlockForever, 2)
  throw("unreachable")
 }
```

2. 如果是异步写，并且chan满了，直接返回

* 使用代码

```go

 c := make(chan string, 4)
 c <- true
 c <- true
 c <- true

 select {
    // 这里
 case c <- true:
 default:
 }
```

* 实现代码

```go
 if !block && c.closed == 0 && full(c) {
  return false
 }
```

3. 如果已经有go程读等待了，唤醒等待的go程，再写

* 使用代码

```go
 c := make(chan bool)

 go func() {
  // 读
  <-c
 }()

 // 写
 c <- true
```

* 实现代码

```go
 if sg := c.recvq.dequeue(); sg != nil {
  // Found a waiting receiver. We pass the value we want to send
  // directly to the receiver, bypassing the channel buffer (if any).
  send(c, sg, ep, func() { unlock(&c.lock) }, 3)
  return true
 }
```

4. 写有缓冲的chan里面写数据

* 使用的代码

```go

 c := make(chan string, 2)
 c <- "hello"
 c <- "world"
 c <- "block"
```

* 实现的代码

```go
 if c.qcount < c.dataqsiz {
  // Space is available in the channel buffer. Enqueue the element to send.
  qp := chanbuf(c, c.sendx)
  if raceenabled {
   racenotify(c, c.sendx, nil)
  }
  typedmemmove(c.elemtype, qp, ep)
  c.sendx++
  if c.sendx == c.dataqsiz {
   c.sendx = 0
  }
  c.qcount++
  unlock(&c.lock)
  return true
 }

```

5. 写无缓冲的chan里面写数据, 没有生产者

* 使用

```go
 c := make(chan bool)
 <-true
```

* 实现

```go
// No stack splits between assigning elem and enqueuing mysg
 // on gp.waiting where copystack can find it.
 mysg.elem = ep
 mysg.waitlink = nil
 mysg.g = gp
 mysg.isSelect = false
 mysg.c = c
 gp.waiting = mysg
 gp.param = nil
 c.sendq.enqueue(mysg)
 // Signal to anyone trying to shrink our stack that we're about
 // to park on a channel. The window between when this G's status
 // changes and when we set gp.activeStackChans is not safe for
 // stack shrinking.
 gp.parkingOnChan.Store(true)
 gopark(chanparkcommit, unsafe.Pointer(&c.lock), waitReasonChanSend, traceBlockChanSend, 2)
 // Ensure the value being sent is kept alive until the
 // receiver copies it out. The sudog has a pointer to the
 // stack object, but sudogs aren't considered as roots of the
 // stack tracer.
 KeepAlive(ep)

 // someone woke us up.
 if mysg != gp.waiting {
  throw("G waiting list is corrupted")
 }
 gp.waiting = nil
 gp.activeStackChans = false
 closed := !mysg.success
 gp.param = nil
 if mysg.releasetime > 0 {
  blockevent(mysg.releasetime-t0, 2)
 }
 mysg.c = nil
 releaseSudog(mysg)
 if closed {
  if c.closed == 0 {
   throw("chansend: spurious wakeup")
  }
  panic(plainError("send on closed channel"))
 }
```

## 二、read流程

1. 读空chan

* 使用

```go
var c chan bool
<-c
```

* 实现

```go
if c == nil {
  if !block {
   return
  }
  gopark(nil, nil, waitReasonChanReceiveNilChan, traceBlockForever, 2)
  throw("unreachable")
 }
```

2. 读有缓存chan

* 实现

```go
c := make(chan bool, 4)
c <- true
<-c
```

* 实现

```go
 if c.qcount > 0 {
  // 直接从队列接收
  qp := chanbuf(c, c.recvx)
  if raceenabled {
   racenotify(c, c.recvx, nil)
  }
  if ep != nil {
   typedmemmove(c.elemtype, ep, qp)
  }
  typedmemclr(c.elemtype, qp)
  c.recvx++
  if c.recvx == c.dataqsiz {
   c.recvx = 0
  }
  c.qcount--
  unlock(&c.lock)
  return true, true
 }
```

大致流程是，如果
数据结构的定义为

```go
// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

// 这个文件包含了Go通道的实现。

// 不变量:
//  至少有一个c.sendq和c.recvq是空的，
//  除非是无缓冲通道且有一个goroutine
//  使用select语句同时阻塞在发送和接收上，
//  在这种情况下，c.sendq和c.recvq的长度仅受
//  select语句大小的限制。
//
// 对于有缓冲通道，还有:
//  c.qcount > 0 意味着 c.recvq 是空的。
//  c.qcount < c.dataqsiz 意味着 c.sendq 是空的。

import (
 "internal/abi"
 "runtime/internal/atomic"
 "runtime/internal/math"
 "unsafe"
)

const (
 maxAlign  = 8
 hchanSize = unsafe.Sizeof(hchan{}) + uintptr(-int(unsafe.Sizeof(hchan{}))&(maxAlign-1))
 debugChan = false
)

type hchan struct {
 qcount   uint           // 队列中的总数据量
 dataqsiz uint           // 循环队列的大小
 buf      unsafe.Pointer // 指向一个大小为dataqsiz的数组
 elemsize uint16
 closed   uint32
 elemtype *_type // 元素类型
 sendx    uint   // 发送索引
 recvx    uint   // 接收索引
 recvq    waitq  // 接收等待队列
 sendq    waitq  // 发送等待队列

 // lock保护hchan中的所有字段，以及几个
 // 阻塞在这个通道上的sudogs的字段。
 //
 // 在持有这个锁的情况下不要改变另一个G的状态
 // （特别是不要唤醒一个G），因为这会导致死锁
 // 与栈收缩。
 lock mutex
}

type waitq struct {
 first *sudog
 last*sudog
}

//go:linkname reflect_makechan reflect.makechan
func reflect_makechan(t *chantype, size int)*hchan {
 return makechan(t, size)
}

func makechan64(t *chantype, size int64)*hchan {
 if int64(int(size)) != size {
  panic(plainError("makechan: size out of range"))
 }

 return makechan(t, int(size))
}

func makechan(t *chantype, size int)*hchan {
 elem := t.Elem

 // 编译器会检查这个，但为了安全起见。
 if elem.Size_>= 1<<16 {
  throw("makechan: invalid channel element type")
 }
 if hchanSize%maxAlign != 0 || elem.Align_ > maxAlign {
  throw("makechan: bad alignment")
 }

 mem, overflow := math.MulUintptr(elem.Size_, uintptr(size))
 if overflow || mem > maxAlloc-hchanSize || size < 0 {
  panic(plainError("makechan: size out of range"))
 }

 // 当buf中存储的元素不包含指针时，Hchan不包含对GC有用的指针。
 // buf指向同一个分配，elemtype是持久的。
 // SudoG的引用来自它们拥有的线程，所以它们不能被收集。
 // TODO(dvyukov,rlh): 重新考虑当收集器可以移动分配对象时。
 var c *hchan
 switch {
 case mem == 0:
  // 队列或元素大小为零。
  c = (*hchan)(mallocgc(hchanSize, nil, true))
  // 竞争检测器使用这个位置进行同步。
  c.buf = c.raceaddr()
 case elem.PtrBytes == 0:
  // 元素不包含指针。
  // 在一个调用中分配hchan和buf。
  c = (*hchan)(mallocgc(hchanSize+mem, nil, true))
  c.buf = add(unsafe.Pointer(c), hchanSize)
 default:
  // 元素包含指针。
  c = new(hchan)
  c.buf = mallocgc(mem, elem, true)
 }

 c.elemsize = uint16(elem.Size_)
 c.elemtype = elem
 c.dataqsiz = uint(size)
 lockInit(&c.lock, lockRankHchan)

 if debugChan {
  print("makechan: chan=", c, "; elemsize=", elem.Size_, "; dataqsiz=", size, "\n")
 }
 return c
}

// chanbuf(c, i) 是指向缓冲区中第i个槽的指针。
func chanbuf(c *hchan, i uint) unsafe.Pointer {
 return add(c.buf, uintptr(i)*uintptr(c.elemsize))
}

// full 报告在c上发送是否会阻塞（即通道已满）。
// 它使用一个单字大小的可变状态读取，所以虽然
// 答案是瞬时的真，但正确的答案可能在调用函数收到返回值时已经改变。
func full(c *hchan) bool {
 // c.dataqsiz是不可变的（在通道创建后从未写入过）
 // 所以在通道操作期间的任何时间读取都是安全的。
 if c.dataqsiz == 0 {
  // 假设指针读取是宽松原子的。
  return c.recvq.first == nil
 }
 // 假设uint读取是宽松原子的。
 return c.qcount == c.dataqsiz
}

// 编译代码中的入口点 for c <- x。
//
//go:nosplit
func chansend1(c *hchan, elem unsafe.Pointer) {
 chansend(c, elem, true, getcallerpc())
}

/*

* 通用单通道发送/接收
* 如果block不为nil，
* 那么协议将不会
* 睡眠，但如果不能
* 完成则返回。
*
* 当涉及的通道在睡眠中被关闭时，睡眠可以以g.param == nil唤醒。
* 最简单的方法是循环并重新运行
* 操作；我们会看到它现在已经关闭了。
 */
func chansend(c*hchan, ep unsafe.Pointer, block bool, callerpc uintptr) bool {
 if c == nil {
  if !block {
   return false
  }
  gopark(nil, nil, waitReasonChanSendNilChan, traceBlockForever, 2)
  throw("unreachable")
 }

 if debugChan {
  print("chansend: chan=", c, "\n")
 }

 if raceenabled {
  racereadpc(c.raceaddr(), callerpc, abi.FuncPCABIInternal(chansend))
 }

 // 快速路径：在不获取锁的情况下检查失败的非阻塞操作。
 //
 // 在观察到通道未关闭后，我们观察到通道
 // 未准备好发送。这些观察都是单字大小的读取
 // （首先是c.closed，其次是full()）。
 // 因为一个关闭的通道不能从“准备好发送”过渡到
 // “未准备好发送”，即使通道在两次观察之间关闭，
 // 它们也意味着在两者之间的一个时刻，通道既未关闭
 // 也未准备好发送。我们表现得好像在那个时刻观察到了通道，
 // 并报告发送不能继续。
 //
 // 如果读取在这里重新排序是可以的：如果我们观察到通道未准备好发送
 // 然后观察到它未关闭，这意味着通道在第一次观察时未关闭。
 // 然而，这里没有任何保证向前进展。我们依赖于chanrecv()和closechan()中的锁释放
 // 副作用来更新这个线程对c.closed和full()的视图。
 if !block && c.closed == 0 && full(c) {
  return false
 }

 var t0 int64
 if blockprofilerate > 0 {
  t0 = cputicks()
 }

 lock(&c.lock)

 if c.closed != 0 {
  unlock(&c.lock)
  panic(plainError("send on closed channel"))
 }

 if sg := c.recvq.dequeue(); sg != nil {
  // 找到一个等待的接收者。我们直接将我们要发送的值
  // 传递给接收者，绕过通道缓冲区（如果有的话）。
  send(c, sg, ep, func() { unlock(&c.lock)}, 3)
  return true
 }

 if c.qcount < c.dataqsiz {
  // 通道缓冲区中有空间。将元素入队发送。
  // 获取sendx对应位置的指针
  qp := chanbuf(c, c.sendx)
  if raceenabled {
   racenotify(c, c.sendx, nil)
  }

  // ep就是待写入元素的指针地址
  // c <- ep
  // typedmemmove约等于 c.buf[sendx] = ep
  // 其中typedmemmove是字节拷贝
  typedmemmove(c.elemtype, qp, ep)
  c.sendx++
  if c.sendx == c.dataqsiz {
   c.sendx = 0
  }
  c.qcount++
  unlock(&c.lock)
  return true
 }

 if !block {
  unlock(&c.lock)
  return false
 }

 // 在通道上阻塞。某个接收者将为我们完成操作。
 gp := getg()
 mysg := acquireSudog()
 mysg.releasetime = 0
 if t0 != 0 {
  mysg.releasetime = -1
 }
 // 在分配elem和将mysg入队到gp.waiting之间没有堆栈分裂
 // 这样copystack可以在找到它时找到它。
 mysg.elem = ep
 mysg.waitlink = nil
 mysg.g = gp
 mysg.isSelect = false
 mysg.c = c
 gp.waiting = mysg
 gp.param = nil
 c.sendq.enqueue(mysg)
 // 向任何试图缩小我们堆栈的人发出信号，我们即将
 // 在通道上停车。当我们改变这个G的状态和设置gp.activeStackChans之间的窗口
 // 对于堆栈收缩是不安全的。
 gp.parkingOnChan.Store(true)
 gopark(chanparkcommit, unsafe.Pointer(&c.lock), waitReasonChanSend, traceBlockChanSend, 2)
 // 确保发送的值保持活动状态，直到接收者复制它。
 // sudog有一个指向堆栈对象的指针，但sudogs不被认为是堆栈跟踪器的根。
 KeepAlive(ep)

 // 有人唤醒了我们。
 if mysg != gp.waiting {
  throw("G waiting list is corrupted")
 }
 gp.waiting = nil
 gp.activeStackChans = false
 closed := !mysg.success
 gp.param = nil
 if mysg.releasetime > 0 {
  blockevent(mysg.releasetime-t0, 2)
 }
 mysg.c = nil
 releaseSudog(mysg)
 if closed {
  if c.closed == 0 {
   throw("chansend: spurious wakeup")
  }
  panic(plainError("send on closed channel"))
 }
 return true
}

// send 处理在空通道c上的发送操作。
// 发送者sg发送的值被复制到接收者sg。
// 然后接收者被唤醒继续其工作。
// 通道c必须为空且被锁定。send用unlockf解锁c。
// sg必须已经从c中出队。
// ep必须非空且指向堆或调用者的堆栈。
func send(c *hchan, sg*sudog, ep unsafe.Pointer, unlockf func(), skip int) {
 if raceenabled {
  if c.dataqsiz == 0 {
   racesync(c, sg)
  } else {
   // 假装我们通过缓冲区，尽管
   // 我们直接复制。注意我们需要在raceenabled时增加
   // 头/尾位置。
   racenotify(c, c.recvx, nil)
   racenotify(c, c.recvx, sg)
   c.recvx++
   if c.recvx == c.dataqsiz {
    c.recvx = 0
   }
   c.sendx = c.recvx // c.sendx = (c.sendx+1) % c.dataqsiz
  }
 }
 if sg.elem != nil {
  sendDirect(c.elemtype, sg, ep)
  sg.elem = nil
 }
 gp := sg.g
 unlockf()
 gp.param = unsafe.Pointer(sg)
 sg.success = true
 if sg.releasetime != 0 {
  sg.releasetime = cputicks()
 }
 goready(gp, skip+1)
}

// Sends and receives on unbuffered or empty-buffered channels are the
// only operations where one running goroutine writes to the stack of
// another running goroutine. The GC assumes that stack writes only
// happen when the goroutine is running and are only done by that
// goroutine. Using a write barrier is sufficient to make up for
// violating that assumption, but the write barrier has to work.
// typedmemmove will call bulkBarrierPreWrite, but the target bytes
// are not in the heap, so that will not help. We arrange to call
// memmove and typeBitsBulkBarrier instead.

func sendDirect(t *_type, sg*sudog, src unsafe.Pointer) {
 // src在我们的堆栈上，dst是另一个堆栈上的槽。

 // 一旦我们从sg中读取sg.elem，如果目标的堆栈被复制（收缩），它将不再
 // 被更新。所以确保在读取和使用之间没有抢占点。
 dst := sg.elem
 typeBitsBulkBarrier(t, uintptr(dst), uintptr(src), t.Size_)
 // 不需要cgo写屏障检查，因为dst总是在Go内存中。
 memmove(dst, src, t.Size_)
}

func recvDirect(t *_type, sg *sudog, dst unsafe.Pointer) {
 // dst在我们的堆栈或堆上，src在另一个堆栈上。
 // 通道被锁定，所以src在操作期间不会移动。
 src := sg.elem
 typeBitsBulkBarrier(t, uintptr(dst), uintptr(src), t.Size_)
 memmove(dst, src, t.Size_)
}

func closechan(c *hchan) {
 if c == nil {
  panic(plainError("close of nil channel"))
 }

 lock(&c.lock)
 if c.closed != 0 {
  unlock(&c.lock)
  panic(plainError("close of closed channel"))
 }

 if raceenabled {
  callerpc := getcallerpc()
  racewritepc(c.raceaddr(), callerpc, abi.FuncPCABIInternal(closechan))
  racerelease(c.raceaddr())
 }

 c.closed = 1

 var glist gList

 // 释放所有接收者
 for {
  sg := c.recvq.dequeue()
  if sg == nil {
   break
  }
  if sg.elem != nil {
   typedmemclr(c.elemtype, sg.elem)
   sg.elem = nil
  }
  if sg.releasetime != 0 {
   sg.releasetime = cputicks()
  }
  gp := sg.g
  gp.param = unsafe.Pointer(sg)
  sg.success = false
  if raceenabled {
   raceacquireg(gp, c.raceaddr())
  }
  glist.push(gp)
 }

 // 释放所有发送者（它们会恐慌）
 for {
  sg := c.sendq.dequeue()
  if sg == nil {
   break
  }
  sg.elem = nil
  if sg.releasetime != 0 {
   sg.releasetime = cputicks()
  }
  gp := sg.g
  gp.param = unsafe.Pointer(sg)
  sg.success = false
  if raceenabled {
   raceacquireg(gp, c.raceaddr())
  }
  glist.push(gp)
 }
 unlock(&c.lock)

 // 现在我们已经释放了通道锁，准备好所有G。
 for !glist.empty() {
  gp := glist.pop()
  gp.schedlink = 0
  goready(gp, 3)
 }
}

// empty 报告从c读取是否会阻塞（即通道为空）。
// 它使用一个单原子读取的可变状态。
func empty(c *hchan) bool {
 // c.dataqsiz是不可变的。
 if c.dataqsiz == 0 {
  return atomic.Loadp(unsafe.Pointer(&c.sendq.first)) == nil
 }
 return atomic.Loaduint(&c.qcount) == 0
}

// 编译代码中的入口点 for <- c。
//
//go:nosplit
func chanrecv1(c *hchan, elem unsafe.Pointer) {
 chanrecv(c, elem, true)
}

//go:nosplit
func chanrecv2(c *hchan, elem unsafe.Pointer) (received bool) {
 _, received = chanrecv(c, elem, true)
 return
}

// chanrecv 在通道c上接收并将其接收到的数据写入ep。
// ep可以为nil，在这种情况下，接收到的数据被忽略。
// 如果block == false且没有可用元素，返回(false, false)。
// 否则，如果c已关闭，将*ep清零并返回(true, false)。
// 否则，将元素填充到*ep并返回(true, true)。
// 非nil的ep必须指向堆或调用者的堆栈。
func chanrecv(c *hchan, ep unsafe.Pointer, block bool) (selected, received bool) {
 // raceenabled: 不需要检查ep，因为它总是在堆栈上
 // 或由reflect新分配的内存。

 if debugChan {
  print("chanrecv: chan=", c, "\n")
 }

 if c == nil {
  if !block {
   return
  }
  gopark(nil, nil, waitReasonChanReceiveNilChan, traceBlockForever, 2)
  throw("unreachable")
 }

 // 快速路径：在不获取锁的情况下检查失败的非阻塞操作。
 if !block && empty(c) {
  // 在观察到通道未准备好接收后，我们观察通道是否关闭。
  // 这些检查的重新排序可能导致不正确的行为。
  // 例如，如果通道是打开且非空，关闭，然后被清空，
  // 重新排序的读取可能错误地指示“打开且空”。为了防止重新排序，
  // 我们对两个检查都使用原子加载，并依赖于清空和关闭在
  // 同一个锁下的单独临界区中发生。这个假设在关闭
  // 一个阻塞发送的无缓冲通道时失败，但那是错误情况。
  if atomic.Load(&c.closed) == 0 {
   // 因为通道不能重新打开，后来的观察通道
   // 未关闭意味着它在第一次观察时也未关闭。
   // 我们表现得好像在那个时刻观察到了通道
   // 并报告接收不能继续。
   return
  }
  // 通道已不可逆转地关闭。重新检查通道是否有任何挂起的数据
  // 在空和关闭检查之间接收，这可能在两者之间到达。
  // 顺序一致性在这里也是必需的，当与这样的发送竞争时。
  if empty(c) {
   // 通道已不可逆转地关闭且空。
   if raceenabled {
    raceacquire(c.raceaddr())
   }
   if ep != nil {
    typedmemclr(c.elemtype, ep)
   }
   return true, false
  }
 }

 var t0 int64
 if blockprofilerate > 0 {
  t0 = cputicks()
 }

 lock(&c.lock)

 if c.closed != 0 {
  if c.qcount == 0 {
   if raceenabled {
    raceacquire(c.raceaddr())
   }
   unlock(&c.lock)
   if ep != nil {
    typedmemclr(c.elemtype, ep)
   }
   return true, false
  }
  // 通道已关闭，但通道缓冲区有数据。
 } else {
  // 刚刚找到未关闭的等待发送者。
  if sg := c.sendq.dequeue(); sg != nil {
   // 找到一个等待的发送者。如果缓冲区大小为0，直接从发送者接收值。
   // 否则，从队列头部接收值并将发送者的值添加到队列尾部（两者映射到
   // 同一个缓冲区槽，因为队列是满的）。
   recv(c, sg, ep, func() { unlock(&c.lock) }, 3)
   return true, true
  }
 }

 if c.qcount > 0 {
  // 直接从队列接收
  qp := chanbuf(c, c.recvx)
  if raceenabled {
   racenotify(c, c.recvx, nil)
  }
  if ep != nil {
   typedmemmove(c.elemtype, ep, qp)
  }
  typedmemclr(c.elemtype, qp)
  c.recvx++
  if c.recvx == c.dataqsiz {
   c.recvx = 0
  }
  c.qcount--
  unlock(&c.lock)
  return true, true
 }

 if !block {
  unlock(&c.lock)
  return false, false
 }

 // 没有发送者可用：在这个通道上阻塞。
 gp := getg()
 mysg := acquireSudog()
 mysg.releasetime = 0
 if t0 != 0 {
  mysg.releasetime = -1
 }
 // 在分配elem和将mysg入队到gp.waiting之间没有堆栈分裂
 // 这样copystack可以在找到它时找到它。
 mysg.elem = ep
 mysg.waitlink = nil
 gp.waiting = mysg
 mysg.g = gp
 mysg.isSelect = false
 mysg.c = c
 gp.param = nil
 c.recvq.enqueue(mysg)
 // 向任何试图缩小我们堆栈的人发出信号，我们即将
 // 在通道上停车。当我们改变这个G的状态和设置gp.activeStackChans之间的窗口
 // 对于堆栈收缩是不安全的。
 gp.parkingOnChan.Store(true)
 gopark(chanparkcommit, unsafe.Pointer(&c.lock), waitReasonChanReceive, traceBlockChanRecv, 2)

 // 有人唤醒了我们
 if mysg != gp.waiting {
  throw("G waiting list is corrupted")
 }
 gp.waiting = nil
 gp.activeStackChans = false
 if mysg.releasetime > 0 {
  blockevent(mysg.releasetime-t0, 2)
 }
 success := mysg.success
 gp.param = nil
 mysg.c = nil
 releaseSudog(mysg)
 return true, success
}

// recv 处理在满通道c上的接收操作。
// 有两个部分：
//  1. 发送者sg发送的值被放入通道
//     并唤醒发送者继续其工作。
//  2. 接收者（当前G）接收的值
//     被写入ep。
//
// 对于同步通道，两个值是相同的。
// 对于异步通道，接收者从
// 通道缓冲区获取数据，发送者的数据被放入
// 通道缓冲区。
// 通道c必须满且被锁定。recv用unlockf解锁c。
// sg必须已经从c中出队。
// 非nil的ep必须指向堆或调用者的堆栈。
func recv(c *hchan, sg*sudog, ep unsafe.Pointer, unlockf func(), skip int) {
 if c.dataqsiz == 0 {
  if raceenabled {
   racesync(c, sg)
  }
  if ep != nil {
   // 从发送者复制数据
   recvDirect(c.elemtype, sg, ep)
  }
 } else {
  // 队列是满的。取队列头部的项目。
  // 让发送者将其项目入队到队列尾部。由于
  // 队列是满的，这两个槽是同一个。
  qp := chanbuf(c, c.recvx)
  if raceenabled {
   racenotify(c, c.recvx, nil)
   racenotify(c, c.recvx, sg)
  }
  // 从队列复制数据到接收者
  if ep != nil {
   typedmemmove(c.elemtype, ep, qp)
  }
  // 从发送者复制数据到队列
  typedmemmove(c.elemtype, qp, sg.elem)
  c.recvx++
  if c.recvx == c.dataqsiz {
   c.recvx = 0
  }
  c.sendx = c.recvx // c.sendx = (c.sendx+1) % c.dataqs
   }
 sg.elem = nil
 gp := sg.g
 unlockf()
 gp.param = unsafe.Pointer(sg)
 sg.success = true
 if sg.releasetime != 0 {
  sg.releasetime = cputicks()
 }
 goready(gp, skip+1)
}

func chanparkcommit(gp *g, chanLock unsafe.Pointer) bool {
 // 有未锁定的sudogs指向gp的堆栈。堆栈
 // 复制必须锁定这些sudogs的通道。
 // 在尝试停车之前设置activeStackChans
 // 因为我们可能在通道锁上自死锁。
 gp.activeStackChans = true
 // 标记现在可以安全地进行堆栈收缩，
 // 因为任何线程获取这个G的堆栈进行收缩
 // 保证在设置activeStackChans后观察到它。
 gp.parkingOnChan.Store(false)
 // 确保在设置activeStackChans和不设置parkingOnChan之后解锁chanLock。
 // 我们解锁chanLock的时刻，我们冒着gp被通道操作唤醒的风险，
 // 所以gp可能在解锁之前看不到一切（甚至gp自己）。
 unlock((*mutex)(chanLock))
 return true
}

// 编译器实现
//
// select {
// case c <- v:
//  ... foo
// default:
//  ... bar
// }
//
// 作为
//
// if selectnbsend(c, v) {
//  ... foo
// } else {
//  ... bar
// }
func selectnbsend(c *hchan, elem unsafe.Pointer) (selected bool) {
 return chansend(c, elem, false, getcallerpc())
}

// 编译器实现
//
// select {
// case v, ok = <-c:
//  ... foo
// default:
//  ... bar
// }
//
// 作为
//
// if selected, ok = selectnbrecv(&v, c); selected {
//  ... foo
// } else {
//  ... bar
// }
func selectnbrecv(elem unsafe.Pointer, c *hchan) (selected, received bool) {
 return chanrecv(c, elem, false)
}

//go:linkname reflect_chansend reflect.chansend0
func reflect_chansend(c *hchan, elem unsafe.Pointer, nb bool) (selected bool) {
 return chansend(c, elem, !nb, getcallerpc())
}

//go:linkname reflect_chanrecv reflect.chanrecv
func reflect_chanrecv(c *hchan, nb bool, elem unsafe.Pointer) (selected bool, received bool) {
 return chanrecv(c, elem, !nb)
}

//go:linkname reflect_chanlen reflect.chanlen
func reflect_chanlen(c *hchan) int {
 if c == nil {
  return 0
 }
 return int(c.qcount)
}

//go:linkname reflectlite_chanlen internal/reflectlite.chanlen
func reflectlite_chanlen(c *hchan) int {
 if c == nil {
  return 0
 }
 return int(c.qcount)
}

//go:linkname reflect_chancap reflect.chancap
func reflect_chancap(c *hchan) int {
 if c == nil {
  return 0
 }
 return int(c.dataqsiz)
}

//go:linkname reflect_chanclose reflect.chanclose
func reflect_chanclose(c *hchan) {
 closechan(c)
}

func (q *waitq) enqueue(sgp*sudog) {
 sgp.next = nil
 x := q.last
 if x == nil {
  sgp.prev = nil
  q.first = sgp
  q.last = sgp
  return
 }
 sgp.prev = x
 x.next = sgp
 q.last = sgp
}

func (q *waitq) dequeue()*sudog {
 for {
  sgp := q.first
  if sgp == nil {
   return nil
  }
  y := sgp.next
  if y == nil {
   q.first = nil
   q.last = nil
  } else {
   y.prev = nil
   q.first = y
   sgp.next = nil // 标记为已移除（见dequeueSudoG）
  }

  // 如果一个goroutine因为select而被放入这个队列，
  // 在另一个case唤醒它和它抓取通道锁之间有一个小窗口。
  // 一旦它有了锁，它会将自己从队列中移除，所以我们不会在这里看到它。
  // 我们使用G结构中的一个标志来告诉我们何时其他人
  // 赢得了信号这个goroutine的竞赛，但goroutine还没有将自己从队列中移除。
  if sgp.isSelect && !sgp.g.selectDone.CompareAndSwap(0, 1) {
   continue
  }

  return sgp
 }
}

func (c *hchan) raceaddr() unsafe.Pointer {
 // 将通道上的读取和写入操作视为
 // 发生在这个地址。避免使用qcount
 // 或dataqsiz的地址，因为len()和cap()内置函数读取
 // 那些地址，我们不希望它们与
 // 操作如close()竞争。
 return unsafe.Pointer(&c.buf)
}

func racesync(c *hchan, sg*sudog) {
 racerelease(chanbuf(c, 0))
 raceacquireg(sg.g, chanbuf(c, 0))
 racereleaseg(sg.g, chanbuf(c, 0))
 raceacquire(chanbuf(c, 0))
}

// 通知竞争检测器发送或接收涉及缓冲区条目idx
// 和通道c或其通信伙伴sg。
// 这个函数处理c.elemsize==0的特殊情况。
func racenotify(c *hchan, idx uint, sg*sudog) {
 // 我们可以传递对应于条目idx的unsafe.Pointer
 // 而不是idx本身。然而，在未来的版本中，
 // 我们可以更好地处理elemsize==0的情况。
 // 一个未来的改进是使用idx调用TSan：
 // 这样，Go将继续不为elemsize==0的通道分配缓冲区条目，
 // 但竞争检测器可以在幕后处理多个
 // 同步对象（每个idx一个）。
 qp := chanbuf(c, idx)
 // 当elemsize==0时，我们不为通道分配完整的缓冲区。
 // 相反，竞争检测器使用c.buf作为唯一的缓冲区条目。
 // 这种简化阻止我们遵循内存模型的happens-before规则（规则在racereleaseacquire中实现）。
 // 相反，我们在c.buf的同步对象中积累happens-before信息。
 if c.elemsize == 0 {
  if sg == nil {
   raceacquire(qp)
   racerelease(qp)
  } else {
   raceacquireg(sg.g, qp)
   racereleaseg(sg.g, qp)
  }
 } else {
  if sg == nil {
   racereleaseacquire(qp)
  } else {
   racereleaseacquireg(sg.g, qp)
  }
 }
}

```
