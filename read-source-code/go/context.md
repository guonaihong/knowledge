### 一、context 保存值和取值

#### 1.1 使用例子

```go
type favContextKey string

 f := func(ctx context.Context, k favContextKey) {
  if v := ctx.Value(k); v != nil {
   fmt.Println("found value:", v)
   return
  }
  fmt.Println("key not found:", k)
 }

 k := favContextKey("language")
 ctx := context.WithValue(context.Background(), k, "Go")

 f(ctx, k)
 f(ctx, favContextKey("color"))
```

#### 1.2 保存值的实现

WithValue, 直接通过valueCtx，保存本层key/value +上层context

```go
func WithValue(parent Context, key, val any) Context {
 if parent == nil {
  panic("cannot create context from nil parent")
 }
 if key == nil {
  panic("nil key")
 }
 if !reflectlite.TypeOf(key).Comparable() {
  panic("key is not comparable")
 }
 return &valueCtx{parent, key, val}
}
```

#### 1.3 查找的代码实现

value是通过一个for循环，从本层的context断言出具体类型，然后不停.Context取出上层context，直接找到值，或者全部遍历完。如果是第三方实现就直接调用`.Value(key)`

```go
func value(c Context, key any) any {
 for {
  switch ctx := c.(type) {
  case *valueCtx:
   if key == ctx.key {
    return ctx.val
   }
   c = ctx.Context
  case *cancelCtx:
   if key == &cancelCtxKey {
    return c
   }
   c = ctx.Context
  case withoutCancelCtx:
   if key == &cancelCtxKey {
    // This implements Cause(ctx) == nil
    // when ctx is created using WithoutCancel.
    return nil
   }
   c = ctx.c
  case *timerCtx:
   if key == &cancelCtxKey {
    return &ctx.cancelCtx
   }
   c = ctx.Context
  case backgroundCtx, todoCtx:
   return nil
  default:
   return c.Value(key)
  }
 }
}

```

#### 1.4 总结

* context 通过包装上层的context，像链表一样，可以将上层的context传递给下层的context。本质上下层context包含了上层的context
* context.WithValue保存值
* ctx.Value 搜索值, 只找它的父兄弟，不会遍历兄弟节点(注意)

### 二、 context cancel和Done

#### 2.1 例子

```go
gen := func(ctx context.Context) <-chan int {
  dst := make(chan int)
  n := 1
  go func() {
   for {
    select {
    case <-ctx.Done():
     return // returning not to leak the goroutine
    case dst <- n:
     n++
    }
   }
  }()
  return dst
 }

 ctx, cancel := context.WithCancel(context.Background())
 defer cancel() // cancel when we are finished consuming integers

 for n := range gen(ctx) {
  fmt.Println(n)
  if n == 5 {
   break
  }
 }
```

#### 2.2 cancel

* 如果是该ctx被取消了，就把children都取消了
* 并且从父context中移除自己
* 一句话描述，close(done), 删子，删自己

```go
func (c *cancelCtx) cancel(removeFromParent bool, err, cause error) {
 if err == nil {
  panic("context: internal error: missing cancel error")
 }
 if cause == nil {
  cause = err
 }
 c.mu.Lock()
 if c.err != nil {
  c.mu.Unlock()
  return // already canceled
 }
 c.err = err
 c.cause = cause
 d, _ := c.done.Load().(chan struct{})
 if d == nil {
  c.done.Store(closedchan)
 } else {
  close(d)
 }
 for child := range c.children {
  // NOTE: acquiring the child's lock while holding parent's lock.
  child.cancel(false, err, cause)
 }
 c.children = nil
 c.mu.Unlock()

 if removeFromParent {
  removeChild(c.Context, c)
 }
}
```

#### 2.3 done

* 如果有就取出done的chan, 没有就初始化一个
* 如果父context不是标准库派生的，单起一个go程监控父/子context退出的事件

```go

func (c *cancelCtx) Done() <-chan struct{} {
 d := c.done.Load()
 if d != nil {
  return d.(chan struct{})
 }
 c.mu.Lock()
 defer c.mu.Unlock()
 d = c.done.Load()
 if d == nil {
  d = make(chan struct{})
  c.done.Store(d)
 }
 return d.(chan struct{})
}
```

#### 2.4 总结

* 如果多个go程调用ctx.Done()，只要cancel()， 都可以即时退立，这是为什么？主要是被了chan的close触发，如果一个chan被close了，那么所有的<-done都会解除阻塞状态

### 三、 propagateCancel 实现

* 检查下父ctx有没有被取消，没有被取消就初始下

```go
func (c *cancelCtx) propagateCancel(parent Context, child canceler) {
 c.Context = parent

 done := parent.Done()
 if done == nil {
  return // parent is never canceled
 }

 select {
 case <-done:
  // parent is already canceled
  child.cancel(false, parent.Err(), Cause(parent))
  return
 default:
 }

 if p, ok := parentCancelCtx(parent); ok {
  // parent is a *cancelCtx, or derives from one.
  p.mu.Lock()
  if p.err != nil {
   // parent has already been canceled
   child.cancel(false, p.err, p.cause)
  } else {
   if p.children == nil {
    p.children = make(map[canceler]struct{})
   }
   p.children[child] = struct{}{}
  }
  p.mu.Unlock()
  return
 }

 if a, ok := parent.(afterFuncer); ok {
  // parent implements an AfterFunc method.
  c.mu.Lock()
  stop := a.AfterFunc(func() {
   child.cancel(false, parent.Err(), Cause(parent))
  })
  c.Context = stopCtx{
   Context: parent,
   stop:    stop,
  }
  c.mu.Unlock()
  return
 }

 goroutines.Add(1)
 go func() {
  select {
  case <-parent.Done():
   child.cancel(false, parent.Err(), Cause(parent))
  case <-child.Done():
  }
 }()
}
```

### 注释代码版本

```go
// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// 版权所有 2014 The Go Authors。保留所有权利。
// 此源代码的使用受BSD风格的许可证管理
// license that can be found in the LICENSE file.

// Package context 定义了 Context 类型，它携带截止时间、
// 取消信号和其他请求作用域的值，跨越API边界和进程之间。
//
// 进入服务器的请求应该创建一个 [Context]，对外发服务器的调用应该接受一个 Context。
// 它们之间的函数调用链必须传播 Context，可以选择使用 [WithCancel]、[WithDeadline]、
// [WithTimeout] 或 [WithValue] 创建派生的 Context。当 Context 被取消时，所有从它派生的 Context 也会被取消。
//
// [WithCancel]、[WithDeadline] 和 [WithTimeout] 函数接受一个 Context（父级）并返回一个派生的 Context（子级）和一个 [CancelFunc]。
// 调用 CancelFunc 会取消子级及其子级，移除父级的对子级的引用，并停止任何相关的定时器。
// 如果没有调用 CancelFunc，则会泄露子级及其子级，直到父级被取消或定时器触发。go vet 工具检查 CancelFuncs 是否在所有控制流路径上被使用。
//
// [WithCancelCause] 函数返回一个 [CancelCauseFunc]，它接受一个错误并将其记录为取消原因。
// 在取消的上下文或其任何子级上调用 [Cause] 可以检索原因。如果没有指定原因，Cause(ctx) 返回与 ctx.Err() 相同的值。
//
// 使用 Contexts 的程序应该遵循这些规则，以保持跨包的接口一致性，并使静态分析工具能够检查上下文传播：
//
// 不要在 struct 类型中存储 Contexts；而是显式地将 Context 传递给每个需要它的函数。上下文应该是第一个参数，通常命名为 ctx：
//
//  func DoSomething(ctx context.Context, arg Arg) error {
//   // ... use ctx ...
//  }
//
// 不要传递 nil [Context]，即使函数允许它。如果你不确定使用哪个 Context，就传递 [context.TODO]。
//
// 仅将上下文值用于跨进程和API传输的请求作用域数据，而不是用于向函数传递可选参数。
//
// 相同的 Context 可以传递给在不同 goroutine 中运行的函数；Context 可以安全地由多个 goroutine 同时使用。
//
// 见 https://blog.golang.org/context 了解使用 Contexts 的服务器示例代码。
package context

import (
 "errors"
 "internal/reflectlite"
 "sync"
 "sync/atomic"
 "time"
)

// A Context 携带一个截止时间、一个取消信号和其他值，跨越 API 边界。
//
// Context 的方法可以被多个 goroutine 同时调用。
type Context interface {
  // Deadline 返回代表代表代表代表代表代表此上下文代表的工作应该被取消的时间。
  // 如果没有设置截止时间，Deadline 返回 ok==false。连续调用 Deadline 返回相同的结果。
 Deadline() (deadline time.Time, ok bool)

 // Done 返回一个当代表此上下文的工作应该被取消时关闭的 channel。
 // 如果此上下文永远不会被取消，Done 可能返回 nil。连续调用 Done 返回相同的值。
 // Done channel 的关闭可能发生在 cancel 函数返回后的异步。
 //
 // WithCancel 安排在调用 cancel 时关闭 Done；
 // WithDeadline 安排在截止时间到期时关闭 Done；
 // WithTimeout 安排在超时到期时关闭 Done。
 //
 // Done 提供用于 select 语句：
 //
 //  // Stream 使用 DoSomething 生成值并将它们发送到 out
 //  // 直到 DoSomething 返回错误或 ctx.Done 被关闭。
 //  func Stream(ctx context.Context, out chan<- Value) error {
 //   for {
 //    v, err := DoSomething(ctx)
 //    if err != nil {
 //     return err
 //    }
 //    select {
 //    case <-ctx.Done():
 //     return ctx.Err()
 //    case out <- v:
 //    }
 //   }
 //  }
 //
 // 见 https://blog.golang.org/pipelines 了解如何使用 Done channel 进行取消的更多示例。
 Done() <-chan struct{}

  // 如果 Done 尚未关闭，Err 返回 nil。
  // 如果 Done 已关闭，Err 返回一个非nil的错误解释为什么：
  // Canceled 如果上下文被取消
  // 或 DeadlineExceeded 如果上下文的截止时间已过。
  // 在 Err 返回一个非nil错误后，连续调用 Err 返回相同的错误。
 Err() error

// Value 返回与此上下文关联的键 key 的值，如果没有值与 key 关联，则返回 nil。
 // 连续调用 Value 与相同的 key 返回相同的结果。
 //
 // 仅将上下文值用于跨进程和 API 边界传输的请求作用域数据，
 // 而不是用于向函数传递可选参数。
 //
 // 一个键标识上下文中的一个特定值。希望在上下文中存储值的函数通常在包中分配一个键作为全局变量，
 // 然后使用该键作为 context.WithValue 和 Context.Value 的参数。一个键可以是任何支持等价性检查的类型；
 // 包应该将键定义为未导出的类型以避免与其他包中定义的键发生冲突。
 //
 // 定义上下文键的包应该提供使用该键存储的值的类型安全访问器：
 //
 //  // Package user 定义了一个 User 类型，该类型存储在上下文中。
 //  package user
 //
 //  import "context"
 //
 //  // User 是存储在上下文中的值的类型。
 //  type User struct {...}
 //
 //  // key 是在此包中定义的键的未导出类型。
 //  // 这防止了与其他包中定义的键的冲突。
 //  type key int
 //
 //  // userKey 是上下文中 user.User 值的键。它是未导出的；
 //  // 客户端使用 user.NewContext 和 user.FromContext 而不是直接使用此键。
 //  var userKey key
 //
 //  // NewContext 返回一个携带值 u 的新上下文。
 //  func NewContext(ctx context.Context, u *User) context.Context {
 //   return context.WithValue(ctx, userKey, u)
 //  }
 //
 //  // FromContext 返回存储在 ctx 中的 User 值（如果有）。
 //  func FromContext(ctx context.Context) (*User, bool) {
 //   u, ok := ctx.Value(userKey).(*User)
 //   return u, ok
 //  }
 Value(key any) any
}

// Canceled 是当上下文被取消时 [Context.Err] 返回的错误。
var Canceled = errors.New("context canceled")

// DeadlineExceeded 是当上下文的截止时间过去时 [Context.Err] 返回的错误。
var DeadlineExceeded error = deadlineExceededError{}

type deadlineExceededError struct{}

func (deadlineExceededError) Error() string   { return "context deadline exceeded" }
func (deadlineExceededError) Timeout() bool   { return true }
func (deadlineExceededError) Temporary() bool { return true }

// An emptyCtx 永远不会被取消，没有值，也没有截止时间。
// 它是 backgroundCtx 和 todoCtx 的共同基础。
type emptyCtx struct{}

func (emptyCtx) Deadline() (deadline time.Time, ok bool) {
 return
}

func (emptyCtx) Done() <-chan struct{} {
 return nil
}

func (emptyCtx) Err() error {
 return nil
}

func (emptyCtx) Value(key any) any {
 return nil
}

type backgroundCtx struct{ emptyCtx }

func (backgroundCtx) String() string {
 return "context.Background"
}

type todoCtx struct{ emptyCtx }

func (todoCtx) String() string {
 return "context.TODO"
}

// Background 返回一个非nil的空 [Context]。它永远不会被取消，没有值，也没有截止时间。
// 它通常由 main 函数、初始化和测试使用，并作为传入请求的顶层 Context。
func Background() Context {
 return backgroundCtx{}
}

// TODO 返回一个非nil的空 [Context]。当不清楚使用哪个 Context 或者它尚不可用时（因为周围的函数尚未扩展以接受 Context 参数），
// 代码应该使用 context.TODO。
func TODO() Context {
 return todoCtx{}
}

// A CancelFunc 告诉操作放弃其工作。
// A CancelFunc 不会等待工作停止。
// A CancelFunc 可以被多个 goroutine 同时调用。第一次调用后，后续调用 CancelFunc 将不执行任何操作。
type CancelFunc func()

// WithCancel 返回一个带有新 Done channel 的父级副本。返回的上下文的 Done channel 在返回的 cancel 函数被调用或
// 父上下文 的 Done channel 被关闭时关闭，以先发生者为准。
// 取消此上下文会释放与其关联的资源，所以一旦在此 [Context] 中运行的操作完成，代码应该调用 cancel。
func WithCancel(parent Context) (ctx Context, cancel CancelFunc) {
 c := withCancel(parent)
 return c, func() { c.cancel(true, Canceled, nil) }
}

// A CancelCauseFunc 表现得像 [CancelFunc]，但还会设置取消原因。
// 这个原因可以通过在取消的上下文或其任何派生上下文上调用 [Cause] 来检索。
//
// 如果上下文已经被取消，CancelCauseFunc 不会设置原因。
// 例如，如果 childContext 从 parentContext 派生：
//   - 如果在 childContext 用 cause2 取消之前 parentContext 用 cause1 取消，
//     那么 Cause(parentContext) == Cause(childContext) == cause1
//   - 如果在 parentContext 用 cause1 取消之前 childContext 用 cause2 取消，
//     那么 Cause(parentContext) == cause1 且 Cause(childContext) == cause2
type CancelCauseFunc func(cause error)

// WithCancelCause 表现得像 [WithCancel]，但返回一个 [CancelCauseFunc] 而不是 [CancelFunc]。
// 使用非nil错误（“原因”）调用 cancel 会在 ctx 中记录该错误；
// 然后可以使用 Cause(ctx) 检索它。
// 使用 nil 调用 cancel 将原因设置为 Canceled。
//
// 示例用法：
//
// ctx, cancel := context.WithCancelCause(parent)
// cancel(myError)
// ctx.Err() // 返回 context.Canceled
// context.Cause(ctx) // 返回 myError
func WithCancelCause(parent Context) (ctx Context, cancel CancelCauseFunc) {
 c := withCancel(parent)
 return c, func(cause error) { c.cancel(true, Canceled, cause) }
}

func withCancel(parent Context) *cancelCtx {
 if parent == nil {
  panic("cannot create context from nil parent")
 }
 c := &cancelCtx{}
 c.propagateCancel(parent, c)
 return c
}

// Cause 返回解释为什么 c 被取消的非nil错误。
// c 或其父级的第一个取消设置了原因。
// 如果那个取消是通过调用 CancelCauseFunc(err) 进行的，那么 [Cause] 返回 err。
// 否则 Cause(c) 返回与 c.Err() 相同的值。
// 如果 c 还没有被取消，Cause 返回 nil。
func Cause(c Context) error {
 if cc, ok := c.Value(&cancelCtxKey).(*cancelCtx); ok {
  cc.mu.Lock()
  defer cc.mu.Unlock()
  return cc.cause
 }

 // 没有 cancelCtxKey 值，因此我们知道 c 不是由 WithCancelCause 创建的某个上下文的后代。
 // 因此，没有特定的返回原因。
 // 如果这不是标准上下文类型之一，
 // 即使它没有原因，它可能仍然有一个错误。
 return c.Err()
}

// AfterFunc 安排在 ctx 完成后（取消或超时）在其自己的 goroutine 中调用 f。
// 如果 ctx 已经完成，AfterFunc 会立即在其自己的 goroutine 中调用 f。
//
// 对一个上下文的多次 AfterFunc 调用独立操作；
// 一个调用不会替换另一个。
//
// 调用返回的 stop 函数停止将 ctx 与 f 关联。
// 如果调用停止了 f 被运行，它返回 true。
// 如果 stop 返回 false，
// 要么上下文已完成且 f 已在其自己的 goroutine 中启动；
// 或者 f 已经被停止。
// stop 函数在返回之前不等待 f 完成。
// 如果调用者需要知道 f 是否已完成，
// 它必须与 f 显式协调。
//
// 如果 ctx 有一个 "AfterFunc(func()) func() bool" 方法，
// AfterFunc 将使用它来安排调用。
func AfterFunc(ctx Context, f func()) (stop func() bool) {
 a := &afterFuncCtx{
  f: f,
 }
 a.cancelCtx.propagateCancel(ctx, a)
 return func() bool {
  stopped := false
  a.once.Do(func() {
   stopped = true
  })
  if stopped {
   a.cancel(true, Canceled, nil)
  }
  return stopped
 }
}

type afterFuncer interface {
 AfterFunc(func()) func() bool
}

type afterFuncCtx struct {
 cancelCtx
 once sync.Once // either starts running f or stops f from running
 f    func()
}

func (a *afterFuncCtx) cancel(removeFromParent bool, err, cause error) {
 a.cancelCtx.cancel(false, err, cause)
 if removeFromParent {
  removeChild(a.Context, a)
 }
 a.once.Do(func() {
  go a.f()
 })
}

// A stopCtx 被用作带有 AfterFunc 注册的父上下文的父上下文。
// 它保存用于取消注册 AfterFunc 的 stop 函数。
type stopCtx struct {
 Context
 stop func() bool
}

// goroutines 计数曾经创建过的 goroutine 数量；用于测试。
var goroutines atomic.Int32

// &cancelCtxKey 是 cancelCtx 返回自身的键。
var cancelCtxKey int

// parentCancelCtx 返回 parent 的底层 *cancelCtx。
// 它通过查找 parent.Value(&cancelCtxKey) 来找到最内层的 *cancelCtx，然后检查
// parent.Done() 是否与该 *cancelCtx 匹配（如果没有，*cancelCtx
// 已经被包装在提供不同完成通道的自定义实现中，此时我们不应该绕过它）。
func parentCancelCtx(parent Context) (*cancelCtx, bool) {
 done := parent.Done()
 if done == closedchan || done == nil {
  return nil, false
 }
 p, ok := parent.Value(&cancelCtxKey).(*cancelCtx)
 if !ok {
  return nil, false
 }
 pdone, _ := p.done.Load().(chan struct{})
 if pdone != done {
  return nil, false
 }
 return p, true
}

// removeChild 从其父上下文中移除上下文。
func removeChild(parent Context, child canceler) {
 if s, ok := parent.(stopCtx); ok {
  s.stop()
  return
 }
 p, ok := parentCancelCtx(parent)
 if !ok {
  return
 }
 p.mu.Lock()
 if p.children != nil {
  delete(p.children, child)
 }
 p.mu.Unlock()
}

// A canceler 是一个可以直接取消的上下文类型。实现是 *cancelCtx 和 *timerCtx。
type canceler interface {
 cancel(removeFromParent bool, err, cause error)
 Done() <-chan struct{}
}

// closedchan 是一个可重用的已关闭通道。
var closedchan = make(chan struct{})

func init() {
 close(closedchan)
}

// A cancelCtx 可以被取消。当被取消时，它也会取消任何实现 canceler 的子级。
type cancelCtx struct {
 Context

 mu       sync.Mutex            // 保护以下字段
 done     atomic.Value          // chan struct{} 类型，按需创建，由第一次 cancel 调用关闭 
 children map[canceler]struct{} // 第一次 cancel 调用将其设置为 nil
 err      error                 // 第一次 cancel 调用将其设置为非nil
 cause    error                 // 第一次 cancel 调用将其设置为非nil
}

func (c *cancelCtx) Value(key any) any {
 if key == &cancelCtxKey {
  return c
 }
 return value(c.Context, key)
}

func (c *cancelCtx) Done() <-chan struct{} {
 d := c.done.Load()
 if d != nil {
  return d.(chan struct{})
 }
 c.mu.Lock()
 defer c.mu.Unlock()
 d = c.done.Load()
 if d == nil {
  d = make(chan struct{})
  c.done.Store(d)
 }
 return d.(chan struct{})
}

func (c *cancelCtx) Err() error {
 c.mu.Lock()
 err := c.err
 c.mu.Unlock()
 return err
}

// propagateCancel 安排当父上下文被取消时子上下文也被取消。
// 它设置 cancelCtx 的父上下文。
func (c *cancelCtx) propagateCancel(parent Context, child canceler) {
 c.Context = parent

 done := parent.Done()
 if done == nil {
  return // 父上下文永远不会被取消
 }

 select {
 case <-done:
  // 父上下文已经被取消
  child.cancel(false, parent.Err(), Cause(parent))
  return
 default:
 }

 if p, ok := parentCancelCtx(parent); ok {
    // 父上下文是一个 *cancelCtx，或者是由一个派生的。
  p.mu.Lock()
  if p.err != nil {
    // 父上下文已经被取消
   child.cancel(false, p.err, p.cause)
  } else {
   if p.children == nil {
    p.children = make(map[canceler]struct{})
   }
   p.children[child] = struct{}{}
  }
  p.mu.Unlock()
  return
 }

 if a, ok := parent.(afterFuncer); ok {
    // 父上下文实现了一个 AfterFunc 方法。
  c.mu.Lock()
  stop := a.AfterFunc(func() {
   child.cancel(false, parent.Err(), Cause(parent))
  })
  c.Context = stopCtx{
   Context: parent,
   stop:    stop,
  }
  c.mu.Unlock()
  return
 }

 goroutines.Add(1)
 go func() {
  select {
  case <-parent.Done():
   child.cancel(false, parent.Err(), Cause(parent))
  case <-child.Done():
  }
 }()
}

type stringer interface {
 String() string
}

func contextName(c Context) string {
 if s, ok := c.(stringer); ok {
  return s.String()
 }
 return reflectlite.TypeOf(c).String()
}

func (c *cancelCtx) String() string {
 return contextName(c.Context) + ".WithCancel"
}

// cancel 关闭 c.done，取消 c 的每个子项，并如果 removeFromParent 为 true，
// 则将其从其父级的子项中移除。
// cancel 将 c.cause 设置为 cause，如果这是 c 第一次被取消。
func (c *cancelCtx) cancel(removeFromParent bool, err, cause error) {
 if err == nil {
  panic("context: internal error: missing cancel error")
 }
 if cause == nil {
  cause = err
 }
 c.mu.Lock()
 if c.err != nil {
  c.mu.Unlock()
  return // 已经被取消
 }
 c.err = err
 c.cause = cause
 d, _ := c.done.Load().(chan struct{})
 if d == nil {
  c.done.Store(closedchan)
 } else {
  close(d)
 }
 for child := range c.children {
    // 注意：在持有父级锁的同时获取子级的锁。
  child.cancel(false, err, cause)
 }
 c.children = nil
 c.mu.Unlock()

 if removeFromParent {
  removeChild(c.Context, c)
 }
}

// WithoutCancel 返回一个副本，当父上下文被取消时，它不会被取消。
// 返回的上下文不返回截止时间或错误，并且其 Done 通道为 nil。
// 在返回的上下文上调用 [Cause] 返回 nil。
func WithoutCancel(parent Context) Context {
 if parent == nil {
  panic("cannot create context from nil parent")
 }
 return withoutCancelCtx{parent}
}

type withoutCancelCtx struct {
 c Context
}

func (withoutCancelCtx) Deadline() (deadline time.Time, ok bool) {
 return
}

func (withoutCancelCtx) Done() <-chan struct{} {
 return nil
}

func (withoutCancelCtx) Err() error {
 return nil
}

func (c withoutCancelCtx) Value(key any) any {
 return value(c, key)
}

func (c withoutCancelCtx) String() string {
 return contextName(c.c) + ".WithoutCancel"
}

// WithDeadline 返回一个父上下文的副本，截止时间调整为不晚于 d。
// 如果父上下文的截止时间已经比 d 早，则 WithDeadline(parent, d) 在语义上等同于 parent。
// 返回的 [Context.Done] 通道在截止时间到期、返回的取消函数被调用或父上下文的 Done 通道被关闭时关闭，以先发生者为准。
//
// 取消此上下文会释放与其关联的资源，所以一旦在此 [Context] 中运行的操作完成，代码应该调用 cancel。
func WithDeadline(parent Context, d time.Time) (Context, CancelFunc) {
 return WithDeadlineCause(parent, d, nil)
}

// WithDeadlineCause 表现得像 [WithDeadline]，但在截止时间超出时也会设置返回的上下文的原因。
// 返回的 [CancelFunc] 不设置原因。
func WithDeadlineCause(parent Context, d time.Time, cause error) (Context, CancelFunc) {
 if parent == nil {
  panic("cannot create context from nil parent")
 }
 if cur, ok := parent.Deadline(); ok && cur.Before(d) {
    // 当前截止时间已经比新的更早。
  return WithCancel(parent)
 }
 c := &timerCtx{
  deadline: d,
 }
 c.cancelCtx.propagateCancel(parent, c)
 dur := time.Until(d)
 if dur <= 0 {
  c.cancel(true, DeadlineExceeded, cause) // 截止时间已经过去
  return c, func() { c.cancel(false, Canceled, nil) }
 }
 c.mu.Lock()
 defer c.mu.Unlock()
 if c.err == nil {
  c.timer = time.AfterFunc(dur, func() {
   c.cancel(true, DeadlineExceeded, cause)
  })
 }
 return c, func() { c.cancel(true, Canceled, nil) }
}

// A timerCtx 携带一个计时器和截止时间。它嵌入了一个 cancelCtx 来实现 Done 和 Err。
// 它通过停止自己的计时器然后委托给 cancelCtx.cancel 来实现取消。
type timerCtx struct {
 cancelCtx
 timer *time.Timer // Under cancelCtx.mu.

 deadline time.Time
}

func (c *timerCtx) Deadline() (deadline time.Time, ok bool) {
 return c.deadline, true
}

func (c *timerCtx) String() string {
 return contextName(c.cancelCtx.Context) + ".WithDeadline(" +
  c.deadline.String() + " [" +
  time.Until(c.deadline).String() + "])"
}

func (c *timerCtx) cancel(removeFromParent bool, err, cause error) {
 c.cancelCtx.cancel(false, err, cause)
 if removeFromParent {
  // 将此 timerCtx 从其父 cancelCtx 的子项中移除。
  removeChild(c.cancelCtx.Context, c)
 }
 c.mu.Lock()
 if c.timer != nil {
  c.timer.Stop()
  c.timer = nil
 }
 c.mu.Unlock()
}

// WithTimeout 返回 WithDeadline(parent, time.Now().Add(timeout))。
//
// 取消此上下文会释放与其关联的资源，所以一旦在此 [Context] 中运行的操作完成，代码应该调用 cancel：
//
// func slowOperationWithTimeout(ctx context.Context) (Result, error) {
//  ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
//  defer cancel() // 如果 slowOperation 在超时到期之前完成，则释放资源
//  return slowOperation(ctx)
// }
func WithTimeout(parent Context, timeout time.Duration) (Context, CancelFunc) {
 return WithDeadline(parent, time.Now().Add(timeout))
}

// WithTimeoutCause 表现得像 [WithTimeout]，但在超时到期时也会设置返回的上下文的原因。
// 返回的 [CancelFunc] 不设置原因。
func WithTimeoutCause(parent Context, timeout time.Duration, cause error) (Context, CancelFunc) {
 return WithDeadlineCause(parent, time.Now().Add(timeout), cause)
}

// WithValue 返回一个父级的副本，在其中与键 key 相关联的值是 val。
//
// 仅将上下文值用于跨进程和API传输的请求作用域数据，
// 而不是用于向函数传递可选参数。
//
// 提供的键必须是可比较的，不应该是类型 string 或任何其他内置类型以避免
// 包使用上下文时发生冲突。使用 WithValue 的用户应该为键定义自己的类型。为了避免在分配给接口{}时分配，
// 上下文键通常是具体类型的 struct{}。或者，导出的上下文键变量的静态类型应该是指针或接口。
func WithValue(parent Context, key, val any) Context {
 if parent == nil {
  panic("cannot create context from nil parent")
 }
 if key == nil {
  panic("nil key")
 }
 if !reflectlite.TypeOf(key).Comparable() {
  panic("key is not comparable")
 }
 return &valueCtx{parent, key, val}
}

// A valueCtx 携带一个键值对。它为该键实现 Value 并委托所有其他调用给嵌入的 Context。
type valueCtx struct {
 Context
 key, val any
}

// stringify 尝试不使用 fmt 将 v 字符串化，因为我们不希望上下文依赖于 unicode 表。这仅用于 *valueCtx.String()。
func stringify(v any) string {
 switch s := v.(type) {
 case stringer:
  return s.String()
 case string:
  return s
 }
 return reflectlite.TypeOf(v).String()
}

func (c *valueCtx) String() string {
 return contextName(c.Context) + ".WithValue(" +
  stringify(c.key) + ", " +
  stringify(c.val) + ")"
}

func (c *valueCtx) Value(key any) any {
 if c.key == key {
  return c.val
 }
 return value(c.Context, key)
}

func value(c Context, key any) any {
 for {
  switch ctx := c.(type) {
  case *valueCtx:
   if key == ctx.key {
    return ctx.val
   }
   c = ctx.Context
  case *cancelCtx:
   if key == &cancelCtxKey {
    return c
   }
   c = ctx.Context
  case withoutCancelCtx:
   if key == &cancelCtxKey {
    // 这实现了当使用 WithoutCancel 创建 ctx 时 Cause(ctx) == nil
    return nil
   }
   c = ctx.c
  case *timerCtx:
   if key == &cancelCtxKey {
    return &ctx.cancelCtx
   }
   c = ctx.Context
  case backgroundCtx, todoCtx:
   return nil
  default:
   return c.Value(key)
  }
 }
}
```
