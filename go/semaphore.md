### 源代码注释版本

```go
// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package semaphore 提供了一个加权信号量的实现。
package semaphore // import "golang.org/x/sync/semaphore"

import (
 "container/list"
 "context"
 "sync"
)

// waiter 结构体表示一个等待信号量的实体，包含所需的权重和准备通道。
type waiter struct {
 n     int64
 ready chan<- struct{} // 当信号量被获取时关闭的通道。
}

// NewWeighted 创建一个新的加权信号量，其最大并发权重为 n。
func NewWeighted(n int64) *Weighted {
 w := &Weighted{size: n}
 return w
}

// Weighted 提供了一种限制对资源并发访问的方法。
// 调用者可以通过给定的权重请求访问。
type Weighted struct {
 size    int64
 cur     int64
 mu      sync.Mutex
 waiters list.List
}

// Acquire 以权重 n 获取信号量，阻塞直到资源可用或 ctx 完成。
// 成功时返回 nil。失败时返回 ctx.Err() 并保持信号量不变。
func (s *Weighted) Acquire(ctx context.Context, n int64) error {
 done := ctx.Done()

 s.mu.Lock()
 select {
 case <-done:
  // ctx 完成已经“发生在”获取信号量之前，
  // 无论是在调用开始前还是在我们等待互斥锁时。
  // 我们宁愿失败，即使我们可以不阻塞地获取互斥锁。
  s.mu.Unlock()
  return ctx.Err()
 default:
 }
 if s.size-s.cur >= n && s.waiters.Len() == 0 {
  // 由于我们持有 s.mu 并且在检查 done 后没有同步，如果
  // ctx 在返回此分支之前完成，它完成必须与这个调用“并发发生” - 它不能“发生在”我们返回之前。
  // 所以，我们总是可以在这里获取。
  s.cur += n
  s.mu.Unlock()
  return nil
 }

 if n > s.size {
  // 不要让其他 Acquire 调用阻塞在一个注定失败的调用上。
  s.mu.Unlock()
  <-done
  return ctx.Err()
 }

 ready := make(chan struct{})
 w := waiter{n: n, ready: ready}
 elem := s.waiters.PushBack(w)
 s.mu.Unlock()

 select {
 case <-done:
  s.mu.Lock()
  select {
  case <-ready:
   // 在我们被取消后获取了信号量。
   // 假装我们没有获取并放回令牌。
   s.cur -= n
   s.notifyWaiters()
  default:
   isFront := s.waiters.Front() == elem
   s.waiters.Remove(elem)
   // 如果我们位于队列前并且还有额外的令牌，通知其他等待者。
   if isFront && s.size > s.cur {
    s.notifyWaiters()
   }
  }
  s.mu.Unlock()
  return ctx.Err()

 case <-ready:
  // 获取了信号量。检查 ctx 是否已经完成。
  // 我们检查 done 通道而不是调用 ctx.Err，因为我们已经有了通道，而 ctx.Err 是 O(n) 的。
  select {
  case <-done:
   s.Release(n)
   return ctx.Err()
  default:
  }
  return nil
 }
}

// TryAcquire 以权重 n 尝试获取信号量而不阻塞。
// 成功时返回 true。失败时返回 false 并保持信号量不变。
func (s *Weighted) TryAcquire(n int64) bool {
 s.mu.Lock()
 success := s.size-s.cur >= n && s.waiters.Len() == 0
 if success {
  s.cur += n
 }
 s.mu.Unlock()
 return success
}

// Release 以权重 n 释放信号量。
func (s *Weighted) Release(n int64) {
 s.mu.Lock()
 s.cur -= n
 if s.cur < 0 {
  s.mu.Unlock()
  panic("semaphore: released more than held")
 }
 s.notifyWaiters()
 s.mu.Unlock()
}

// notifyWaiters 通知所有等待信号量的等待者。
func (s *Weighted) notifyWaiters() {
 for {
  next := s.waiters.Front()
  if next == nil {
   break // 没有更多的等待者阻塞。
  }

  w := next.Value.(waiter)
  if s.size-s.cur < w.n {
   // 没有足够的令牌给下一个等待者。我们可以继续（尝试找到请求更小的等待者），
   // 但在负载下这可能导致大请求的饥饿；相反，我们让所有剩余的等待者阻塞。
   break
  }

  s.cur += w.n
  s.waiters.Remove(next)
  close(w.ready)
 }
}
```
