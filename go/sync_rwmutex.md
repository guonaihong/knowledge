### 分析实现

* RLock: 主要是基于原子变量判断有没有写锁，如果有写锁就等待(readerSem)，没有写锁就是原子加readCount
* RUnlock: 主是是原子减readCount, 如果readWait为0， 就唤醒写锁(writerSem)
* Lock: readCount加上 -rwmutexMaxReaders, 如果readerWait 大于0， 就等待(writerSem) 被唤醒
* Unlock: readerCount加上 rwmutexMaxReaders, 如果有读者， 就唤醒(readerSem)

### 源代码加注释

```go
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sync

import (
 "internal/race"
 "sync/atomic"
 "unsafe"
)

// 此文件在runtime/rwmutex.go中有一个修改过的副本。
// 如果在此处进行任何更改，请检查是否也应该在那里进行更改。

// RWMutex是一个读/写互斥锁。
// 锁可以被任意数量的读取者或单个写入者持有。
// RWMutex的零值是一个未锁定的互斥锁。
//
// RWMutex在首次使用后不得被复制。
//
// 如果有任何goroutine调用Lock，而锁已经被一个或多个读取者持有，
// 那么并发调用RLock将会阻塞，直到写入者获取（并释放）锁，以确保锁最终可用于写入者。
// 注意，这禁止了递归的读锁。
//
// 在Go内存模型的术语中，
// 第n次调用Unlock“同步于”第m次调用Lock，对于任何n < m，就像Mutex一样。
// 对于任何RLock调用，存在一个n，使得
// 第n次调用Unlock“同步于”该RLock调用，
// 并且相应的RUnlock调用“同步于”第n+1次调用Lock。
type RWMutex struct {
 w           Mutex        // 如果有待处理的写入者，则持有
 writerSem   uint32       // 写入者等待完成读取者的信号量
 readerSem   uint32       // 读取者等待完成写入者的信号量
 readerCount atomic.Int32 // 待处理的读取者数量
 readerWait  atomic.Int32 // 离开的读取者数量
}

const rwmutexMaxReaders = 1 << 30

// 通过以下方式向竞争检测器指示发生之前的关系：
// - Unlock  -> Lock:  readerSem
// - Unlock  -> RLock: readerSem
// - RUnlock -> Lock:  writerSem
//
// 下面的方法暂时禁用处理竞争同步事件，
// 以便向竞争检测器提供上述更精确的模型。
//
// 例如，RLock中的atomic.AddInt32不应提供
// 获取-释放语义，这会错误地同步竞争的读取者，从而可能错过竞争。

// RLock锁定rw以进行读取。
//
// 不应将其用于递归读锁；阻塞的Lock调用会阻止新读取者获取锁。请参阅RWMutex类型的文档。
func (rw *RWMutex) RLock() {
 if race.Enabled {
  _ = rw.w.state
  race.Disable()
 }
 if rw.readerCount.Add(1) < 0 {
  // 有待处理的写入者，等待它。
  runtime_SemacquireRWMutexR(&rw.readerSem, false, 0)
 }
 if race.Enabled {
  race.Enable()
  race.Acquire(unsafe.Pointer(&rw.readerSem))
 }
}

// TryRLock尝试锁定rw以进行读取，并报告是否成功。
//
// 注意，虽然存在正确的TryRLock用法，但它们很少见，
// 使用TryRLock通常表明在互斥锁的特定使用中存在更深层次的问题。
func (rw *RWMutex) TryRLock() bool {
 if race.Enabled {
  _ = rw.w.state
  race.Disable()
 }
 for {
  c := rw.readerCount.Load()
  if c < 0 {
   if race.Enabled {
    race.Enable()
   }
   return false
  }
  if rw.readerCount.CompareAndSwap(c, c+1) {
   if race.Enabled {
    race.Enable()
    race.Acquire(unsafe.Pointer(&rw.readerSem))
   }
   return true
  }
 }
}

// RUnlock撤销单个RLock调用；
// 它不影响其他同时读取者。
// 如果rw未锁定以进行读取，则进入RUnlock是运行时错误。
func (rw *RWMutex) RUnlock() {
 if race.Enabled {
  _ = rw.w.state
  race.ReleaseMerge(unsafe.Pointer(&rw.writerSem))
  race.Disable()
 }
 if r := rw.readerCount.Add(-1); r < 0 {
  // 概述的慢路径，以允许快速路径内联
  rw.rUnlockSlow(r)
 }
 if race.Enabled {
  race.Enable()
 }
}

func (rw *RWMutex) rUnlockSlow(r int32) {
 if r+1 == 0 || r+1 == -rwmutexMaxReaders {
  race.Enable()
  fatal("sync: RUnlock of unlocked RWMutex")
 }
 // 有待处理的写入者。
 if rw.readerWait.Add(-1) == 0 {
  // 最后一个读取者解除了写入者的阻塞。
  runtime_Semrelease(&rw.writerSem, false, 1)
 }
}

// Lock锁定rw以进行写入。
// 如果锁已经被锁定以进行读取或写入，
// Lock会阻塞，直到锁可用。
func (rw *RWMutex) Lock() {
 if race.Enabled {
  _ = rw.w.state
  race.Disable()
 }
 // 首先，解决与其他写入者的竞争。
 rw.w.Lock()
 // 向读取者宣布有待处理的写入者。
 r := rw.readerCount.Add(-rwmutexMaxReaders) + rwmutexMaxReaders
 // 等待活跃的读取者。
 if r != 0 && rw.readerWait.Add(r) != 0 {
  runtime_SemacquireRWMutex(&rw.writerSem, false, 0)
 }
 if race.Enabled {
  race.Enable()
  race.Acquire(unsafe.Pointer(&rw.readerSem))
  race.Acquire(unsafe.Pointer(&rw.writerSem))
 }
}

// TryLock尝试锁定rw以进行写入，并报告是否成功。
//
// 注意，虽然存在正确的TryLock用法，但它们很少见，
// 使用TryLock通常表明在互斥锁的特定使用中存在更深层次的问题。
func (rw *RWMutex) TryLock() bool {
 if race.Enabled {
  _ = rw.w.state
  race.Disable()
 }
 if !rw.w.TryLock() {
  if race.Enabled {
   race.Enable()
  }
  return false
 }
 if !rw.readerCount.CompareAndSwap(0, -rwmutexMaxReaders) {
  rw.w.Unlock()
  if race.Enabled {
   race.Enable()
  }
  return false
 }
 if race.Enabled {
  race.Enable()
  race.Acquire(unsafe.Pointer(&rw.readerSem))
  race.Acquire(unsafe.Pointer(&rw.writerSem))
 }
 return true
}

// Unlock解锁rw以进行写入。如果rw未锁定以进行写入，则进入Unlock是运行时错误。
//
// 与Mutex一样，锁定的RWMutex不与特定goroutine关联。一个goroutine可以RLock（Lock）一个RWMutex，然后
// 安排另一个goroutine来RUnlock（Unlock）它。
func (rw *RWMutex) Unlock() {
 if race.Enabled {
  _ = rw.w.state
  race.Release(unsafe.Pointer(&rw.readerSem))
  race.Disable()
 }

 // 向读取者宣布没有活跃的写入者。
 r := rw.readerCount.Add(rwmutexMaxReaders)
 if r >= rwmutexMaxReaders {
  race.Enable()
  fatal("sync: Unlock of unlocked RWMutex")
 }
 // 解除阻塞的读取者，如果有的话。
 for i := 0; i < int(r); i++ {
  runtime_Semrelease(&rw.readerSem, false, 0)
 }
 // 允许其他写入者继续。
 rw.w.Unlock()
 if race.Enabled {
  race.Enable()
 }
}

// syscall_hasWaitingReaders报告是否有任何goroutine正在等待
// 获取对rw的读锁。这是因为syscall.ForkLock是一个RWMutex，
// 我们不能在不破坏兼容性的情况下更改它。我们不需要也不想要ForkLock的RWMutex语义，
// 我们使用这个私有API来避免必须更改ForkLock的类型。
// 有关更多详细信息，请参阅syscall包。
//
//go:linkname syscall_hasWaitingReaders syscall.hasWaitingReaders
func syscall_hasWaitingReaders(rw *RWMutex) bool {
 r := rw.readerCount.Load()
 return r < 0 && r+rwmutexMaxReaders > 0
}

// RLocker返回一个Locker接口，该接口通过调用rw.RLock和rw.RUnlock实现
// Lock和Unlock方法。
func (rw *RWMutex) RLocker() Locker {
 return (*rlocker)(rw)
}

type rlocker RWMutex

func (r *rlocker) Lock()   { (*RWMutex)(r).RLock() }
func (r *rlocker) Unlock() { (*RWMutex)(r).RUnlock() }
```
