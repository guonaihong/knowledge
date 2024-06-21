### select

### select 编译器优化

* 一个select 写

```go
// 改写前
select {
    case ch <-i:
}

// 改写后
if chansend1(ch, i) {
    
}
```

* 一个select 写和default调用的是selectnbsend

```go
// 改写前
select {
case ch <- i:
    ...
default:
    ...
}

// 改写后
if selectnbsend(ch, i) {
    ...
} else {
    ...
}
```

* 一个select 读

```go
// 改写前
select {
    case v <- ch: // case v, ok <- ch:
}

// 改写后
if chanrecv1() {

}
```

* 一个select 读和default调用的是selectnbrecv

```go
// 改写前
select {
case v <- ch: // case v, ok <- ch:
    ......
default:
    ......
}

// 改写后
if selectnbrecv(&v, ch) { // if selectnbrecv2(&v, &ok, ch) {
    ...
} else {
    ...
}
```

多个select调用的是selectgo方法

* 先打扰排序
* 随便挑一个执行

```go
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

// This file contains the implementation of Go select statements.

import (
 "internal/abi"
 "unsafe"
)

const debugSelect = false

// Select case descriptor.
// Known to compiler.
// Changes here must also be made in src/cmd/compile/internal/walk/select.go's scasetype.
type scase struct {
 c    *hchan         // 通道
 elem unsafe.Pointer // 数据元素
}

var (
 chansendpc = abi.FuncPCABIInternal(chansend)
 chanrecvpc = abi.FuncPCABIInternal(chanrecv)
)

func selectsetpc(pc *uintptr) {
 *pc = getcallerpc()
}

func sellock(scases []scase, lockorder []uint16) {
 var c *hchan
 for _, o := range lockorder {
  c0 := scases[o].c
  if c0 != c {
   c = c0
   lock(&c.lock)
  }
 }
}

func selunlock(scases []scase, lockorder []uint16) {
 // 我们必须非常小心，不要在解锁最后一个锁之后接触 sel，因为 sel 可能会被立即释放。
 // 考虑以下情况。
 // 第一个 M 在 runtime·selectgo() 中调用 runtime·park() 传递 sel。
 // 一旦 runtime·park() 解锁了最后一个锁，另一个 M 使调用 select 的 G 再次可运行并调度它执行。
 // 当 G 在另一个 M 上运行时，它锁定所有锁并释放 sel。
 // 现在如果第一个 M 接触 sel，它将访问已释放的内存。
 for i := len(lockorder) - 1; i >= 0; i-- {
  c := scases[lockorder[i]].c
  if i > 0 && c == scases[lockorder[i-1]].c {
   continue // 将在下一次迭代中解锁
  }
  unlock(&c.lock)
 }
}

func selparkcommit(gp *g, _ unsafe.Pointer) bool {
 // 有未锁定的 sudog 指向 gp 的栈。栈复制必须锁定这些 sudog 的通道。
 // 在尝试停车之前设置 activeStackChans，而不是在设置 activeStackChans 和取消设置 parkingOnChan 之后解锁。
 gp.activeStackChans = true
 // 标记现在可以安全地进行栈收缩，因为任何试图收缩这个 G 的栈的线程都保证在设置 activeStackChans 之后观察到它。
 gp.parkingOnChan.Store(false)
 // 确保在设置 activeStackChans 和取消设置 parkingOnChan 之后解锁。
 // 在我们解锁任何通道锁的那一刻，我们可能会被通道操作唤醒，因此 gp 可能会在解锁之前的一切都可见之前继续运行（甚至对 gp 本身也是如此）。

 // 这不能访问 gp 的栈（见 gopark）。特别是，它不能访问 *hselect。
 // 这没关系，因为当调用这个函数时，gp.waiting 已经按锁顺序包含了所有通道。
 var lastc *hchan
 for sg := gp.waiting; sg != nil; sg = sg.waitlink {
  if sg.c != lastc && lastc != nil {
   // 一旦我们解锁通道，任何 sudog 中的字段都可能改变，包括 c 和 waitlink。
   // 由于多个 sudog 可能具有相同的通道，我们只在通过最后一个通道实例后解锁。
   unlock(&lastc.lock)
  }
  lastc = sg.c
 }
 if lastc != nil {
  unlock(&lastc.lock)
 }
 return true
}

func block() {
 gopark(nil, nil, waitReasonSelectNoCases, traceBlockForever, 1) // 永远等待
}

// selectgo 实现 select 语句。
//
// cas0 指向一个类型为 [ncases]scase 的数组，order0 指向一个类型为 [2*ncases]uint16 的数组，其中 ncases 必须 <= 65536。
// 两者都位于 goroutine 的栈上（无论 selectgo 中是否有逃逸）。
//
// 对于 race detector 构建，pc0 指向一个类型为 [ncases]uintptr 的数组（也在栈上）；对于其他构建，它被设置为 nil。
//
// selectgo 返回所选 scase 的索引，该索引与其相应的 select{recv,send,default} 调用的序号位置匹配。
// 此外，如果所选的 scase 是接收操作，它报告是否接收到值。
func selectgo(cas0 *scase, order0 *uint16, pc0 *uintptr, nsends, nrecvs int, block bool) (int, bool) {
 if debugSelect {
  print("select: cas0=", cas0, "\n")
 }

 // 注意：为了保持一个精简的栈大小，scase 的数量上限为 65536。
 cas1 := (*[1 << 16]scase)(unsafe.Pointer(cas0))
 order1 := (*[1 << 17]uint16)(unsafe.Pointer(order0))

 ncases := nsends + nrecvs
 scases := cas1[:ncases:ncases]
 pollorder := order1[:ncases:ncases]
 lockorder := order1[ncases:][:ncases:ncases]
 // 注意：pollorder/lockorder 的基础数组未被编译器零初始化。

 // 即使 raceenabled 为 true，也可能有在不带 -race 编译的包中编译的 select 语句（例如，runtime/signal_unix.go 中的 ensureSigM）。
 var pcs []uintptr
 if raceenabled && pc0 != nil {
  pc1 := (*[1 << 16]uintptr)(unsafe.Pointer(pc0))
  pcs = pc1[:ncases:ncases]
 }
 casePC := func(casi int) uintptr {
  if pcs == nil {
   return 0
  }
  return pcs[casi]
 }

 var t0 int64
 if blockprofilerate > 0 {
  t0 = cputicks()
 }

 // 编译器将静态只有 0 或 1 个 case 加上 default 的 select 重写为更简单的结构。
 // 我们在这里遇到的唯一情况是较大的 select 中大多数通道已被置空。通用代码正确处理这些情况，并且它们很少见，不值得优化（和测试）。

 // 生成随机顺序
 norder := 0
 for i := range scases {
  cas := &scases[i]

  // 从 poll 和 lock 顺序中省略没有通道的 case。
  if cas.c == nil {
   cas.elem = nil // 允许 GC
   continue
  }

  j := cheaprandn(uint32(norder + 1))
  pollorder[norder] = pollorder[j]
  pollorder[j] = uint16(i)
  norder++
 }
 pollorder = pollorder[:norder]
 lockorder = lockorder[:norder]

 // 按 Hchan 地址对 case 进行排序以获得锁定顺序。
 // 简单的堆排序，保证 n log n 时间和恒定的栈空间。
 for i := range lockorder {
  j := i
  // 从 pollorder 开始以在同一通道上排列 case。
  c := scases[pollorder[i]].c
  for j > 0 && scases[lockorder[(j-1)/2]].c.sortkey() < c.sortkey() {
   k := (j - 1) / 2
   lockorder[j] = lockorder[k]
   j = k
  }
  lockorder[j] = pollorder[i]
 }
 for i := len(lockorder) - 1; i >= 0; i-- {
  o := lockorder[i]
  c := scases[o].c
  lockorder[i] = lockorder[0]
  j := 0
  for {
   k := j*2 + 1
   if k >= i {
    break
   }
    if k+1 < i && scases[lockorder[k]].c.sortkey() < scases[lockorder[k+1]].c.sortkey() {
               k++
   }
   if c.sortkey() < scases[lockorder[k]].c.sortkey() {
    lockorder[j] = lockorder[k]
    j = k
    continue
   }
   break
  }
  lockorder[j] = o
 }

 if debugSelect {
  for i := 0; i+1 < len(lockorder); i++ {
   if scases[lockorder[i]].c.sortkey() > scases[lockorder[i+1]].c.sortkey() {
    print("i=", i, " x=", lockorder[i], " y=", lockorder[i+1], "\n")
    throw("select: broken sort")
   }
  }
 }

 // 锁定所有涉及的通道
 sellock(scases, lockorder)

 var (
  gp     *g
  sg     *sudog
  c      *hchan
  k      *scase
  sglist *sudog
  sgnext *sudog
  qp     unsafe.Pointer
  nextp  **sudog
 )

 // 第一遍 - 查找已经等待的操作
 var casi int
 var cas *scase
 var caseSuccess bool
 var caseReleaseTime int64 = -1
 var recvOK bool
 for _, casei := range pollorder {
  casi = int(casei)
  cas = &scases[casi]
  c = cas.c

  if casi >= nsends {
   sg = c.sendq.dequeue()
   if sg != nil {
    goto recv
   }
   if c.qcount > 0 {
    goto bufrecv
   }
   if c.closed != 0 {
    goto rclose
   }
  } else {
   if raceenabled {
    racereadpc(c.raceaddr(), casePC(casi), chansendpc)
   }
   if c.closed != 0 {
    goto sclose
   }
   sg = c.recvq.dequeue()
   if sg != nil {
    goto send
   }
   if c.qcount < c.dataqsiz {
    goto bufsend
   }
  }
 }

 if !block {
  selunlock(scases, lockorder)
  casi = -1
  goto retc
 }

 // 第二遍 - 在所有通道上排队
 gp = getg()
 if gp.waiting != nil {
  throw("gp.waiting != nil")
 }
 nextp = &gp.waiting
 for _, casei := range lockorder {
  casi = int(casei)
  cas = &scases[casi]
  c = cas.c
  sg := acquireSudog()
  sg.g = gp
  sg.isSelect = true
  // 在将 elem 分配和将 sg 入队到 gp.waiting 之间没有栈分割，copystack 可以找到它。
  sg.elem = cas.elem
  sg.releasetime = 0
  if t0 != 0 {
   sg.releasetime = -1
  }
  sg.c = c
  // 按锁顺序构造等待列表。
  *nextp = sg
  nextp = &sg.waitlink

  if casi < nsends {
   c.sendq.enqueue(sg)
  } else {
   c.recvq.enqueue(sg)
  }
 }

 // 等待被唤醒
 gp.param = nil
 // 向任何试图收缩我们栈的线程发出信号，我们即将在通道上停车。收缩栈的窗口在我们设置 gp.activeStackChans 和取消设置 gp.parkingOnChan 之间是不安全的。
 gp.parkingOnChan.Store(true)
 gopark(selparkcommit, nil, waitReasonSelect, traceBlockSelect, 1)
 gp.activeStackChans = false

 sellock(scases, lockorder)

 gp.selectDone.Store(0)
 sg = (*sudog)(gp.param)
 gp.param = nil

 // 第三遍 - 从不成功的通道中出队
 // 否则它们会在安静的通道上堆积
 // 记录成功的 case，如果有的话。
 // 我们按锁顺序单链了 SudoGs。
 casi = -1
 cas = nil
 caseSuccess = false
 sglist = gp.waiting
 // 在解除链接之前清除所有 elem。
 for sg1 := gp.waiting; sg1 != nil; sg1 = sg1.waitlink {
  sg1.isSelect = false
  sg1.elem = nil
  sg1.c = nil
 }
 gp.waiting = nil

 for _, casei := range lockorder {
  k = &scases[casei]
  if sg == sglist {
   // sg 已经被唤醒我们的 G 出队。
   casi = int(casei)
   cas = k
   caseSuccess = sglist.success
   if sglist.releasetime > 0 {
    caseReleaseTime = sglist.releasetime
   }
  } else {
   c = k.c
   if int(casei) < nsends {
    c.sendq.dequeueSudoG(sglist)
   } else {
    c.recvq.dequeueSudoG(sglist)
   }
  }
  sgnext = sglist.waitlink
  sglist.waitlink = nil
  releaseSudog(sglist)
  sglist = sgnext
 }

 if cas == nil {
  throw("selectgo: bad wakeup")
 }

 c = cas.c

 if debugSelect {
  print("wait-return: cas0=", cas0, " c=", c, " cas=", cas, " send=", casi < nsends, "\n")
 }

 if casi < nsends {
  if !caseSuccess {
   goto sclose
  }
 } else {
  recvOK = caseSuccess
 }

 if raceenabled {
  if casi < nsends {
   raceReadObjectPC(c.elemtype, cas.elem, casePC(casi), chansendpc)
  } else if cas.elem != nil {
   raceWriteObjectPC(c.elemtype, cas.elem, casePC(casi), chanrecvpc)
  }
 }
 if msanenabled {
  if casi < nsends {
   msanread(cas.elem, c.elemtype.Size_)
  } else if cas.elem != nil {
   msanwrite(cas.elem, c.elemtype.Size_)
  }
 }
 if asanenabled {
  if casi < nsends {
   asanread(cas.elem, c.elemtype.Size_)
  } else if cas.elem != nil {
   asanwrite(cas.elem, c.elemtype.Size_)
  }
 }

 selunlock(scases, lockorder)
 goto retc

bufrecv:
 // 可以从缓冲区接收
 if raceenabled {
  if cas.elem != nil {
   raceWriteObjectPC(c.elemtype, cas.elem, casePC(casi), chanrecvpc)
  }
  racenotify(c, c.recvx, nil)
 }
 if msanenabled && cas.elem != nil {
  msanwrite(cas.elem, c.elemtype.Size_)
 }
 if asanenabled && cas.elem != nil {
  asanwrite(cas.elem, c.elemtype.Size_)
 }
 recvOK = true
 qp = chanbuf(c, c.recvx)
 if cas.elem != nil {
  typedmemmove(c.elemtype, cas.elem, qp)
 }
 typedmemclr(c.elemtype, qp)
 c.recvx++
 if c.recvx == c.dataqsiz {
  c.recvx = 0
 }
 c.qcount--
 selunlock(scases, lockorder)
 goto retc

bufsend:
 // 可以发送到缓冲区
 if raceenabled {
  racenotify(c, c.sendx, nil)
  raceReadObjectPC(c.elemtype, cas.elem, casePC(casi), chansendpc)
 }
 if msanenabled {
  msanread(cas.elem, c.elemtype.Size_)

         if asanenabled {
  asanread(cas.elem, c.elemtype.Size_)
 }
 typedmemmove(c.elemtype, chanbuf(c, c.sendx), cas.elem)
 c.sendx++
 if c.sendx == c.dataqsiz {
  c.sendx = 0
 }
 c.qcount++
 selunlock(scases, lockorder)
 goto retc

recv:
 // 可以从睡眠的发送者 (sg) 接收
 recv(c, sg, cas.elem, func() { selunlock(scases, lockorder) }, 2)
 if debugSelect {
  print("syncrecv: cas0=", cas0, " c=", c, "\n")
 }
 recvOK = true
 goto retc

rclose:
 // 从关闭的通道末尾读取
 selunlock(scases, lockorder)
 recvOK = false
 if cas.elem != nil {
  typedmemclr(c.elemtype, cas.elem)
 }
 if raceenabled {
  raceacquire(c.raceaddr())
 }
 goto retc

send:
 // 可以发送到睡眠的接收者 (sg)
 if raceenabled {
  raceReadObjectPC(c.elemtype, cas.elem, casePC(casi), chansendpc)
 }
 if msanenabled {
  msanread(cas.elem, c.elemtype.Size_)
 }
 if asanenabled {
  asanread(cas.elem, c.elemtype.Size_)
 }
 send(c, sg, cas.elem, func() { selunlock(scases, lockorder) }, 2)
 if debugSelect {
  print("syncsend: cas0=", cas0, " c=", c, "\n")
 }
 goto retc

retc:
 if caseReleaseTime > 0 {
  blockevent(caseReleaseTime-t0, 1)
 }
 return casi, recvOK

sclose:
 // 发送到关闭的通道
 selunlock(scases, lockorder)
 panic(plainError("send on closed channel"))
}

func (c *hchan) sortkey() uintptr {
 return uintptr(unsafe.Pointer(c))
}

// A runtimeSelect is a single case passed to rselect.
// This must match ../reflect/value.go:/runtimeSelect
type runtimeSelect struct {
 dir selectDir
 typ unsafe.Pointer // channel type (not used here)
 ch  *hchan         // channel
 val unsafe.Pointer // ptr to data (SendDir) or ptr to receive buffer (RecvDir)
}

// These values must match ../reflect/value.go:/SelectDir.
type selectDir int

const (
 _             selectDir = iota
 selectSend              // case Chan <- Send
 selectRecv              // case <-Chan:
 selectDefault           // default
)

//go:linkname reflect_rselect reflect.rselect
func reflect_rselect(cases []runtimeSelect) (int, bool) {
 if len(cases) == 0 {
  block()
 }
 sel := make([]scase, len(cases))
 orig := make([]int, len(cases))
 nsends, nrecvs := 0, 0
 dflt := -1
 for i, rc := range cases {
  var j int
  switch rc.dir {
  case selectDefault:
   dflt = i
   continue
  case selectSend:
   j = nsends
   nsends++
  case selectRecv:
   nrecvs++
   j = len(cases) - nrecvs
  }

  sel[j] = scase{c: rc.ch, elem: rc.val}
  orig[j] = i
 }

 // 只有 default case。
 if nsends+nrecvs == 0 {
  return dflt, false
 }

 // 如果必要，压缩 sel 和 orig。
 if nsends+nrecvs < len(cases) {
  copy(sel[nsends:], sel[len(cases)-nrecvs:])
  copy(orig[nsends:], orig[len(cases)-nrecvs:])
 }

 order := make([]uint16, 2*(nsends+nrecvs))
 var pc0 *uintptr
 if raceenabled {
  pcs := make([]uintptr, nsends+nrecvs)
  for i := range pcs {
   selectsetpc(&pcs[i])
  }
  pc0 = &pcs[0]
 }

 chosen, recvOK := selectgo(&sel[0], &order[0], pc0, nsends, nrecvs, dflt == -1)

 // 将 chosen 转换回调用者的顺序。
 if chosen < 0 {
  chosen = dflt
 } else {
  chosen = orig[chosen]
 }
 return chosen, recvOK
}

func (q *waitq) dequeueSudoG(sgp *sudog) {
 x := sgp.prev
 y := sgp.next
 if x != nil {
  if y != nil {
   // 队列中间
   x.next = y
   y.prev = x
   sgp.next = nil
   sgp.prev = nil
   return
  }
  // 队列末尾
  x.next = nil
  q.last = x
  sgp.prev = nil
  return
 }
 if y != nil {
  // 队列开头
  y.prev = nil
  q.first = y
  sgp.next = nil
  return
 }

 // x==y==nil. 要么 sgp 是队列中唯一的元素，要么它已经被移除。使用 q.first 来消除歧义。
 if q.first == sgp {
  q.first = nil
  q.last = nil
 }
}
```

### 参考资料

<https://draveness.me/golang/docs/part2-foundation/ch05-keyword/golang-select/>
