### 代码加注释版本

```go
package breaker

import (
 "context"
 "errors"
 "fmt"
 "strings"
 "sync"
 "time"

 "github.com/zeromicro/go-zero/core/mathx"
 "github.com/zeromicro/go-zero/core/proc"
 "github.com/zeromicro/go-zero/core/stat"
 "github.com/zeromicro/go-zero/core/stringx"
)

const (
 numHistoryReasons = 5 // 历史错误原因的数量
 timeFormat        = "15:04:05" // 时间格式
)

// ErrServiceUnavailable 当断路器状态为打开时返回的错误
var ErrServiceUnavailable = errors.New("circuit breaker is open")

type (
 // Acceptable 用于检查错误是否可以被接受
 Acceptable func(err error) bool

 // Breaker 表示一个断路器
 Breaker interface {
  // Name 返回断路器的名称
  Name() string

  // Allow 检查请求是否被允许
  // 如果允许，返回一个承诺，否则返回ErrServiceUnavailable错误
  // 调用者需要在成功时调用promise.Accept()，在失败时调用promise.Reject()
  Allow() (Promise, error)
  // AllowCtx 在ctx未完成时检查请求是否被允许
  AllowCtx(ctx context.Context) (Promise, error)

  // Do 运行给定的请求，如果断路器接受它
  // 如果断路器拒绝请求，立即返回错误
  // 如果在请求中发生panic，断路器将其视为错误并再次引发相同的panic
  Do(req func() error) error
  // DoCtx 在ctx未完成时运行给定的请求
  DoCtx(ctx context.Context, req func() error) error

  // DoWithAcceptable 运行给定的请求，如果断路器接受它
  // 如果断路器拒绝请求，立即返回错误
  // 如果在请求中发生panic，断路器将其视为错误并再次引发相同的panic
  // acceptable 检查是否是一个成功的调用，即使错误不为nil
  DoWithAcceptable(req func() error, acceptable Acceptable) error
  // DoWithAcceptableCtx 在ctx未完成时运行给定的请求
  DoWithAcceptableCtx(ctx context.Context, req func() error, acceptable Acceptable) error

  // DoWithFallback 运行给定的请求，如果断路器接受它
  // 如果断路器拒绝请求，运行fallback
  // 如果在请求中发生panic，断路器将其视为错误并再次引发相同的panic
  DoWithFallback(req func() error, fallback Fallback) error
  // DoWithFallbackCtx 在ctx未完成时运行给定的请求
  DoWithFallbackCtx(ctx context.Context, req func() error, fallback Fallback) error

  // DoWithFallbackAcceptable 运行给定的请求，如果断路器接受它
  // 如果断路器拒绝请求，运行fallback
  // 如果在请求中发生panic，断路器将其视为错误并再次引发相同的panic
  // acceptable 检查是否是一个成功的调用，即使错误不为nil
  DoWithFallbackAcceptable(req func() error, fallback Fallback, acceptable Acceptable) error
  // DoWithFallbackAcceptableCtx 在ctx未完成时运行给定的请求
  DoWithFallbackAcceptableCtx(ctx context.Context, req func() error, fallback Fallback,
   acceptable Acceptable) error
 }

 // Fallback 是在请求被拒绝时调用的函数
 Fallback func(err error) error

 // Option 定义了自定义断路器的方法
 Option func(breaker *circuitBreaker)

 // Promise 接口定义了Breaker.Allow返回的回调
 Promise interface {
  // Accept 告诉断路器调用成功
  Accept()
  // Reject 告诉断路器调用失败
  Reject(reason string)
 }

 internalPromise interface {
  Accept()
  Reject()
 }

 circuitBreaker struct {
  name string
  throttle
 }

 internalThrottle interface {
  allow() (internalPromise, error)
  doReq(req func() error, fallback Fallback, acceptable Acceptable) error
 }

 throttle interface {
  allow() (Promise, error)
  doReq(req func() error, fallback Fallback, acceptable Acceptable) error
 }
)

// NewBreaker 返回一个Breaker对象
// opts可以用于自定义断路器
func NewBreaker(opts ...Option) Breaker {
 var b circuitBreaker
 for _, opt := range opts {
  opt(&b)
 }
 if len(b.name) == 0 {
  b.name = stringx.Rand()
 }
 b.throttle = newLoggedThrottle(b.name, newGoogleBreaker())

 return &b
}

func (cb *circuitBreaker) Allow() (Promise, error) {
 return cb.throttle.allow()
}

func (cb *circuitBreaker) AllowCtx(ctx context.Context) (Promise, error) {
 select {
 case <-ctx.Done():
  return nil, ctx.Err()
 default:
  return cb.Allow()
 }
}

func (cb *circuitBreaker) Do(req func() error) error {
 return cb.throttle.doReq(req, nil, defaultAcceptable)
}

func (cb *circuitBreaker) DoCtx(ctx context.Context, req func() error) error {
 select {
 case <-ctx.Done():
  return ctx.Err()
 default:
  return cb.Do(req)
 }
}

func (cb *circuitBreaker) DoWithAcceptable(req func() error, acceptable Acceptable) error {
 return cb.throttle.doReq(req, nil, acceptable)
}

func (cb *circuitBreaker) DoWithAcceptableCtx(ctx context.Context, req func() error,
 acceptable Acceptable) error {
 select {
 case <-ctx.Done():
  return ctx.Err()
 default:
  return cb.DoWithAcceptable(req, acceptable)
 }
}

func (cb *circuitBreaker) DoWithFallback(req func() error, fallback Fallback) error {
 return cb.throttle.doReq(req, fallback, defaultAcceptable)
}

func (cb *circuitBreaker) DoWithFallbackCtx(ctx context.Context, req func() error,
 fallback Fallback) error {
 select {
 case <-ctx.Done():
  return ctx.Err()
 default:
  return cb.DoWithFallback(req, fallback)
 }
}

func (cb *circuitBreaker) DoWithFallbackAcceptable(req func() error, fallback Fallback,
 acceptable Acceptable) error {
 return cb.throttle.doReq(req, fallback, acceptable)
}

func (cb *circuitBreaker) DoWithFallbackAcceptableCtx(ctx context.Context, req func() error,
 fallback Fallback, acceptable Acceptable) error {
 select {
 case <-ctx.Done():
  return ctx.Err()
 default:
  return cb.DoWithFallbackAcceptable(req, fallback, acceptable)
 }
}

func (cb *circuitBreaker) Name() string {
 return cb.name
}

// WithName 返回一个设置断路器名称的函数
func WithName(name string) Option {
 return func(b *circuitBreaker) {
  b.name = name
 }
}

func defaultAcceptable(err error) bool {
 return err == nil
}

type loggedThrottle struct {
 name string
 internalThrottle
 errWin *errorWindow
}

func newLoggedThrottle(name string, t internalThrottle) loggedThrottle {
 return loggedThrottle{
  name:             name,
  internalThrottle: t,
  errWin:           new(errorWindow),
 }
}

func (lt loggedThrottle) allow() (Promise, error) {
 promise, err := lt.internalThrottle.allow()
 return promiseWithReason{
  promise: promise,
  errWin:  lt.errWin,
 }, lt.logError(err)
}

func (lt loggedThrottle) doReq(req func() error, fallback Fallback, acceptable Acceptable) error {
 return lt.logError(lt.internalThrottle.doReq(req, fallback, func(err error) bool {
  accept := acceptable(err)
  if !accept && err != nil {
   lt.errWin.add(err.Error())
  }
  return accept
 }))
}

func (lt loggedThrottle) logError(err error) error {
 if errors.Is(err, ErrServiceUnavailable) {
  // 如果断路器打开，不可能有空的错误窗口
  stat.Report(fmt.Sprintf(
   "proc(%s/%d), callee: %s, breaker is open and requests dropped\nlast errors:\n%s",
   proc.ProcessName(), proc.Pid(), lt.name, lt.errWin))
 }

 return err
}

type errorWindow struct {
 reasons [numHistoryReasons]string
 index   int
 count   int
 lock    sync.Mutex
}

func (ew *errorWindow) add(reason string) {
 ew.lock.Lock()
 ew.reasons[ew.index] = fmt.Sprintf("%s %s", time.Now().Format(timeFormat), reason)
 ew.index = (ew.index + 1) % numHistoryReasons
 ew.count = mathx.MinInt(ew.count+1, numHistoryReasons)
 ew.lock.Unlock()
}

func (ew *errorWindow) String() string {
 var reasons []string

 ew.lock.Lock()
 // 反向顺序
 for i := ew.index - 1; i >= ew.index-ew.count; i-- {
  reasons = append(reasons, ew.reasons[(i+numHistoryReasons)%numHistoryReasons])
 }
 ew.lock.Unlock()

 return strings.Join(reasons, "\n")
}

type promiseWithReason struct {
 promise internalPromise
 errWin  *errorWindow
}

func (p promiseWithReason) Accept() {
 p.promise.Accept()
}

func (p promiseWithReason) Reject(reason string) {
 p.errWin.add(reason)
 p.promise.Reject()
}
```
