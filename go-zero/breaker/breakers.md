```go
package breaker

import (
 "context"
 "sync"
)

var (
 lock     sync.RWMutex // 读写锁，用于保护breakers map的并发访问
 breakers = make(map[string]Breaker) // 存储断路器实例的map
)

// Do 调用具有给定名称的Breaker的Do方法
func Do(name string, req func() error) error {
 return do(name, func(b Breaker) error {
  return b.Do(req)
 })
}

// DoCtx 调用具有给定名称的Breaker的DoCtx方法
func DoCtx(ctx context.Context, name string, req func() error) error {
 return do(name, func(b Breaker) error {
  return b.DoCtx(ctx, req)
 })
}

// DoWithAcceptable 调用具有给定名称的Breaker的DoWithAcceptable方法
func DoWithAcceptable(name string, req func() error, acceptable Acceptable) error {
 return do(name, func(b Breaker) error {
  return b.DoWithAcceptable(req, acceptable)
 })
}

// DoWithAcceptableCtx 调用具有给定名称的Breaker的DoWithAcceptableCtx方法
func DoWithAcceptableCtx(ctx context.Context, name string, req func() error,
 acceptable Acceptable) error {
 return do(name, func(b Breaker) error {
  return b.DoWithAcceptableCtx(ctx, req, acceptable)
 })
}

// DoWithFallback 调用具有给定名称的Breaker的DoWithFallback方法
func DoWithFallback(name string, req func() error, fallback Fallback) error {
 return do(name, func(b Breaker) error {
  return b.DoWithFallback(req, fallback)
 })
}

// DoWithFallbackCtx 调用具有给定名称的Breaker的DoWithFallbackCtx方法
func DoWithFallbackCtx(ctx context.Context, name string, req func() error, fallback Fallback) error {
 return do(name, func(b Breaker) error {
  return b.DoWithFallbackCtx(ctx, req, fallback)
 })
}

// DoWithFallbackAcceptable 调用具有给定名称的Breaker的DoWithFallbackAcceptable方法
func DoWithFallbackAcceptable(name string, req func() error, fallback Fallback,
 acceptable Acceptable) error {
 return do(name, func(b Breaker) error {
  return b.DoWithFallbackAcceptable(req, fallback, acceptable)
 })
}

// DoWithFallbackAcceptableCtx 调用具有给定名称的Breaker的DoWithFallbackAcceptableCtx方法
func DoWithFallbackAcceptableCtx(ctx context.Context, name string, req func() error,
 fallback Fallback, acceptable Acceptable) error {
 return do(name, func(b Breaker) error {
  return b.DoWithFallbackAcceptableCtx(ctx, req, fallback, acceptable)
 })
}

// GetBreaker 返回具有给定名称的Breaker
func GetBreaker(name string) Breaker {
 lock.RLock()
 b, ok := breakers[name]
 lock.RUnlock()
 if ok {
  return b
 }

 lock.Lock()
 b, ok = breakers[name]
 if !ok {
  b = NewBreaker(WithName(name))
  breakers[name] = b
 }
 lock.Unlock()

 return b
}

// NoBreakerFor 禁用具有给定名称的断路器
func NoBreakerFor(name string) {
 lock.Lock()
 breakers[name] = NopBreaker()
 lock.Unlock()
}

// do 执行具有给定名称的Breaker的操作
func do(name string, execute func(b Breaker) error) error {
 return execute(GetBreaker(name))
}
```
