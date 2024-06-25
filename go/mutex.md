### 源代码加注释

### Lock

1. 先通过cash操作将state设置为mutexLocked
2. 如果不行，则进入慢速路径， 先自旋。
3. 使用 runtime_SemacquireMutex(&m.sema, queueLifo, 1) 这个函数等待锁

### Unlock

1. 直接减去mutexLocked这个状态
1. 正常模式，直接使用cas解锁。
1. 饥饿模式，直接信号量唤醒

```go
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sync提供了基本的同步原语，如互斥锁。
// 除了Once和WaitGroup类型之外，大多数都是为低级库例程设计的。
// 更高级别的同步最好通过通道和通信来实现。
//
// 包含此包中定义的类型的值不应被复制。
package sync

import (
 "internal/race"
 "sync/atomic"
 "unsafe"
)

// 由运行时通过linkname提供。
func throw(string)
func fatal(string)

// Mutex是一种互斥锁。
// Mutex的零值是一个未锁定的互斥锁。
//
// Mutex在使用后不得被复制。
//
// 在Go内存模型的术语中，第n次调用Unlock“同步于”第m次调用Lock
// 对于任何n < m。
// 成功的TryLock调用等同于Lock调用。
// 失败的TryLock调用根本不建立任何“同步于”关系。
type Mutex struct {
 state int32
 sema  uint32
}

// Locker表示可以锁定和解锁的对象。
type Locker interface {
 Lock()
 Unlock()
}

const (
 mutexLocked = 1 << iota // 互斥锁被锁定

 // 这个常量表示互斥锁是否处于“唤醒”状态。当一个等待的goroutine被唤醒并准备获取锁时，这个标志被设置。
 // 这通常发生在互斥锁的Unlock方法中，当它决定唤醒一个等待的goroutine时。
 // 这个标志的目的是告诉下一个尝试释放锁的goroutine不需要再唤醒其他等待的goroutine，因为已经有一个正在准备获取锁。
 mutexWoken

 // 这个常量表示互斥锁是否处于“饥饿”模式。
 // 在饥饿模式下，锁的所有权直接从解锁的goroutine传递给等待队列中的第一个goroutine，而不是让新到达的goroutine尝试获取锁。
 // 这有助于防止等待的goroutine长时间得不到执行，从而减少锁的等待时间。
 mutexStarving

 // 这个常量表示等待者计数器在Mutex状态字段中的位移量。
 // Mutex的状态字段是一个组合字段，包含了锁是否被锁定、是否处于饥饿模式、是否有等待者以及是否有唤醒的goroutine等信息。
 // mutexWaiterShift定义了等待者计数器在状态字段中的起始位，用于计算等待者的数量。 
 mutexWaiterShift = iota

 // 互斥锁的公平性。
 //
 // 互斥锁可以在两种操作模式下运行：正常和饥饿。
 // 在正常模式下，等待者按FIFO顺序排队，但唤醒的等待者
 // 不拥有互斥锁，并与新到达的goroutine竞争所有权。
 // 新到达的goroutine具有优势 - 它们已经在CPU上运行，并且可能有很多，
 // 因此唤醒的等待者有很大机会失败。在这种情况下，它被排在等待队列的前面。
 // 如果等待者未能获取互斥锁超过1毫秒，则将互斥锁切换到饥饿模式。
 //
 // 在饥饿模式下，互斥锁的所有权直接从解锁的goroutine传递给
 // 队列前面的等待者。新到达的goroutine不会尝试获取互斥锁，
 // 即使它看起来已解锁，也不会尝试自旋。相反，它们将自己排队在等待队列的尾部。
 //
 // 如果等待者收到互斥锁的所有权，并且看到它要么是队列中的最后一个等待者，
 // 要么等待时间少于1毫秒，则将互斥锁切换回正常操作模式。
 //
 // 正常模式具有更好的性能，因为goroutine可以连续多次获取互斥锁，
 // 即使存在阻塞的等待者。饥饿模式对于防止尾部延迟的病态情况很重要。
 starvationThresholdNs = 1e6
)

// Lock锁定m。
// 如果锁已被使用，则调用goroutine将阻塞，直到互斥锁可用。
func (m *Mutex) Lock() {
 // 快速路径：抓住未锁定的互斥锁。
 if atomic.CompareAndSwapInt32(&m.state, 0, mutexLocked) {
  if race.Enabled {
   race.Acquire(unsafe.Pointer(m))
  }
  return
 }
 // 慢速路径（轮廓化，以便快速路径可以内联）
 m.lockSlow()
}

// TryLock尝试锁定m并报告是否成功。
//
// 注意，虽然存在正确的TryLock用法，但它们很少见，
// 并且使用TryLock通常表明在互斥锁的特定使用中存在更深层次的问题。
func (m *Mutex) TryLock() bool {
 old := m.state
 if old&(mutexLocked|mutexStarving) != 0 {
  return false
 }

 // 可能有goroutine正在等待互斥锁，但我们现在正在运行，
 // 可以尝试在那个goroutine醒来之前抓住互斥锁。
 if !atomic.CompareAndSwapInt32(&m.state, old, old|mutexLocked) {
  return false
 }

 if race.Enabled {
  race.Acquire(unsafe.Pointer(m))
 }
 return true
}

func (m *Mutex) lockSlow() {
 var waitStartTime int64
 starving := false
 awoke := false
 iter := 0
 old := m.state
 for {
  // 不要在饥饿模式下自旋，所有权直接交给等待者，
  // 所以我们无论如何都无法获取互斥锁。
  if old&(mutexLocked|mutexStarving) == mutexLocked && runtime_canSpin(iter) {
   // 主动自旋有意义。
   // 尝试设置mutexWoken标志以通知Unlock
   // 不要唤醒其他阻塞的goroutine。
   if !awoke && old&mutexWoken == 0 && old>>mutexWaiterShift != 0 &&
    atomic.CompareAndSwapInt32(&m.state, old, old|mutexWoken) {
    awoke = true
   }
   runtime_doSpin()
   iter++
   old = m.state
   continue
  }
  new := old
  // 不要尝试获取饥饿的互斥锁，新到达的goroutine必须排队。
  if old&mutexStarving == 0 {
   new |= mutexLocked
  }
  if old&(mutexLocked|mutexStarving) != 0 {
   new += 1 << mutexWaiterShift
  }
  // 当前goroutine将互斥锁切换到饥饿模式。
  // 但如果互斥锁当前未锁定，则不要进行切换。
  // Unlock期望饥饿的互斥锁有等待者，在这种情况下将不成立。
  if starving && old&mutexLocked != 0 {
   new |= mutexStarving
  }
  if awoke {
   // goroutine已从睡眠中唤醒，
   // 因此我们需要在任何情况下重置标志。
   if new&mutexWoken == 0 {
    throw("sync: inconsistent mutex state")
   }
   new &^= mutexWoken
  }
  if atomic.CompareAndSwapInt32(&m.state, old, new) {
   if old&(mutexLocked|mutexStarving) == 0 {
    break // 使用CAS锁定互斥锁
   }
   // 如果我们之前已经在等待，则在队列的前面排队。
   queueLifo := waitStartTime != 0
   if waitStartTime == 0 {
    waitStartTime = runtime_nanotime()
   }
   // queueLifo 为true时，插入到队列的头部
   runtime_SemacquireMutex(&m.sema, queueLifo, 1)
   // 超过1ms
   starving = starving || runtime_nanotime()-waitStartTime > starvationThresholdNs
   old = m.state
   if old&mutexStarving != 0 {
    // 如果此goroutine被唤醒并且互斥锁处于饥饿模式，
    // 所有权已移交给我们，但互斥锁处于某种不一致的状态：
    // mutexLocked未设置，我们仍被视为等待者。修复它。
    if old&(mutexLocked|mutexWoken) != 0 || old>>mutexWaiterShift == 0 {
     throw("sync: inconsistent mutex state")
    }
    delta := int32(mutexLocked - 1<<mutexWaiterShift)
    if !starving || old>>mutexWaiterShift == 1 {
     // 退出饥饿模式。
     // 在这里考虑等待时间是至关重要的。
     // 饥饿模式非常低效，一旦它们将互斥锁切换到饥饿模式，
     // 两个goroutine就可以无限地锁定步骤。
     delta -= mutexStarving
    }
    atomic.AddInt32(&m.state, delta)
    break
   }
   awoke = true
   iter = 0
  } else {
   old = m.state
  }
 }

 if race.Enabled {
  race.Acquire(unsafe.Pointer(m))
 }
}

// Unlock解锁m。
// 如果m在进入Unlock时未锁定，则这是一个运行时错误。
//
// 锁定Mutex与特定goroutine无关。
// 允许一个goroutine锁定Mutex，然后安排另一个goroutine解锁它。
func (m *Mutex) Unlock() {
 if race.Enabled {
  _ = m.state
  race.Release(unsafe.Pointer(m))
 }

 // 快速路径：丢弃锁定位。
 new := atomic.AddInt32(&m.state, -mutexLocked)
 if new != 0 {
  // 轮廓化的慢速路径，以允许内联快速路径。
  // 为了在跟踪期间隐藏unlockSlow，我们在跟踪GoUnblock时跳过一个额外的帧。
  m.unlockSlow(new)
 }
}

func (m *Mutex) unlockSlow(new int32) {
 if (new+mutexLocked)&mutexLocked == 0 {
  fatal("sync: unlock of unlocked mutex")
 }
 if new&mutexStarving == 0 {
  old := new
  for {
   // 如果没有等待者或goroutine已经
   // 被唤醒或抓住了锁，则无需唤醒任何人。
   // 在饥饿模式下，所有权直接从解锁的
   // goroutine传递给下一个等待者。我们不在这个链上，
   // 因为我们解锁互斥锁时没有观察到mutexStarving。
   // 所以让开。
   if old>>mutexWaiterShift == 0 || old&(mutexLocked|mutexWoken|mutexStarving) != 0 {
    return
   }
   // 抓住唤醒某人的权利。
   new = (old - 1<<mutexWaiterShift) | mutexWoken
   if atomic.CompareAndSwapInt32(&m.state, old, new) {
    runtime_Semrelease(&m.sema, false, 1)
    return
   }
   old = m.state
  }
 } else {
  // 饥饿模式：将互斥锁所有权移交给下一个等待者，并让出
  // 我们的时间片，以便下一个等待者可以立即开始运行。
  // 注意：mutexLocked未设置，等待者在唤醒后将设置它。
  // 但如果设置了mutexStarving，则互斥锁仍被视为锁定，
  // 因此新到达的goroutine不会获取它。
  runtime_Semrelease(&m.sema, true, 1)
 }
}
```
