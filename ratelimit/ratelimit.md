#### uber-go的漏桶算法

这个漏桶的实现，把时间分成n等份，通过本次与上次时间差的比较，如果小于一个时间片就sleep下，然后获得令牌，否则就直接获得令牌。
<https://github.com/uber-go/ratelimit>

* 用法

```go
rl := ratelimit.New(100) // per second

prev := time.Now()
for i := 0; i < 10; i++ {
    now := rl.Take()
    fmt.Println(i, now.Sub(prev))
    prev = now
}
```

```go
package ratelimit // import "go.uber.org/ratelimit"

import (
 "sync"
 "time"
)

// mutexLimiter 是一个基于互斥锁的限流器，用于控制请求的速率。
type mutexLimiter struct {
 sync.Mutex // 互斥锁，用于保护共享资源的并发访问
 last       time.Time // 上次请求的时间
 sleepFor   time.Duration // 需要休眠的时间，以保持请求速率
 perRequest time.Duration // 每个请求允许的最小时间间隔
 maxSlack   time.Duration // 允许的最大负休眠时间，用于平滑请求速率
 clock      Clock // 时钟接口，用于获取当前时间和休眠
}

// newMutexBased 创建一个新的基于互斥锁的限流器。
func newMutexBased(rate int, opts ...Option) *mutexLimiter {
 // 构建配置
 config := buildConfig(opts)
 // 计算每个请求的时间间隔
 perRequest := config.per / time.Duration(rate)
 // 创建并初始化mutexLimiter
 l := &mutexLimiter{
  perRequest: perRequest,
  maxSlack:   -1 * time.Duration(config.slack) * perRequest,
  clock:      config.clock,
 }
 return l
}

// Take 方法确保在多次调用Take之间的时间平均为per/rate。
func (t *mutexLimiter) Take() time.Time {
 t.Lock() // 加锁以保护共享资源
 defer t.Unlock() // 解锁

 now := t.clock.Now() // 获取当前时间

 // 如果是第一次请求，则允许
 if t.last.IsZero() {
  t.last = now
  return t.last
 }

 // sleepFor 计算基于每个请求预算和上次请求所花费的时间，我们应该休眠多久
 t.sleepFor += t.perRequest - now.Sub(t.last)

 // 不应允许sleepFor过于负数，因为这意味着服务在短时间内大幅减速后，将获得更高的RPS
 if t.sleepFor < t.maxSlack {
  t.sleepFor = t.maxSlack
 }

 // 如果sleepFor是正数，则现在应该休眠
 if t.sleepFor > 0 {
  t.clock.Sleep(t.sleepFor) // 休眠
  t.last = now.Add(t.sleepFor) // 更新上次请求时间
  t.sleepFor = 0 // 重置sleepFor
 } else {
  t.last = now // 更新上次请求时间
 }

 return t.last // 返回上次请求的时间
}
```

### 令牌桶算法

```go
package rate

import (
 "context"
 "fmt"
 "math"
 "sync"
 "time"
)

// Limit 定义了某些事件的最大频率。
// Limit 表示为每秒的事件数。
// 零值的 Limit 不允许任何事件。
type Limit float64

// Inf 是无限速率限制；它允许所有事件（即使 burst 为零）。
const Inf = Limit(math.MaxFloat64)

// Every 将事件之间的最小时间间隔转换为 Limit。
func Every(interval time.Duration) Limit {
 if interval <= 0 {
  return Inf
 }
 return 1 / Limit(interval.Seconds())
}

// Limiter 控制事件发生的频率。
// 它实现了一个大小为 b 的“令牌桶”，最初是满的，并以每秒 r 个令牌的速度重新填充。
// 非正式地说，在足够大的时间间隔内，Limiter 将速率限制为每秒 r 个令牌，最大突发大小为 b 个事件。
// 作为特例，如果 r == Inf（无限速率），则忽略 b。
// 有关令牌桶的更多信息，请参见 https://en.wikipedia.org/wiki/Token_bucket。
//
// 零值是一个有效的 Limiter，但它将拒绝所有事件。
// 使用 NewLimiter 创建非零的 Limiters。
//
// Limiter 有三个主要方法，Allow、Reserve 和 Wait。
// 大多数调用者应该使用 Wait。
//
// 这三个方法都消耗一个令牌。
// 它们在无可用令牌时的行为不同。
// 如果没有可用的令牌，Allow 返回 false。
// 如果没有可用的令牌，Reserve 返回一个未来令牌的预订以及调用者必须等待使用它的时间。
// 如果没有可用的令牌，Wait 阻塞直到可以获得一个令牌或其关联的 context.Context 被取消。
//
// 方法 AllowN、ReserveN 和 WaitN 消耗 n 个令牌。
//
// Limiter 对多个 goroutine 的并发使用是安全的。
type Limiter struct {
 mu       sync.Mutex
 limit    Limit
 burst    int
 tokens   float64
 // last 是 limiter 的 tokens 字段最后一次更新的时间
 last     time.Time
 // lastEvent 是速率限制事件（过去或未来）的最新时间
 lastEvent time.Time
}

// Limit 返回最大整体事件速率。
func (lim *Limiter) Limit() Limit {
 lim.mu.Lock()
 defer lim.mu.Unlock()
 return lim.limit
}

// Burst 返回最大突发大小。Burst 是在单个调用 Allow、Reserve 或 Wait 中可以消耗的最大令牌数，因此更高的 Burst 值允许更多的事件一次性发生。
// 零 Burst 不允许任何事件，除非 limit == Inf。
func (lim *Limiter) Burst() int {
 lim.mu.Lock()
 defer lim.mu.Unlock()
 return lim.burst
}

// TokensAt 返回在时间 t 时可用的令牌数量。
func (lim *Limiter) TokensAt(t time.Time) float64 {
 lim.mu.Lock()
 _, tokens := lim.advance(t) // 不改变 lim
 lim.mu.Unlock()
 return tokens
}

// Tokens 返回当前可用的令牌数量。
func (lim *Limiter) Tokens() float64 {
 return lim.TokensAt(time.Now())
}

// NewLimiter 返回一个新的 Limiter，它允许速率 r 的事件，并允许最多 b 个令牌的突发。
func NewLimiter(r Limit, b int) *Limiter {
 return &Limiter{
  limit: r,
  burst: b,
 }
}

// Allow 报告事件现在是否可以发生。
func (lim *Limiter) Allow() bool {
 return lim.AllowN(time.Now(), 1)
}

// AllowN 报告在时间 t 时 n 个事件是否可以发生。
// 如果您打算丢弃/跳过超过速率限制的事件，请使用此方法。否则使用 Reserve 或 Wait。
func (lim *Limiter) AllowN(t time.Time, n int) bool {
 return lim.reserveN(t, n, 0).ok
}

// Reservation 包含有关由 Limiter 允许在延迟后发生的事件的信息。
// Reservation 可以被取消，这可能允许 Limiter 允许额外的事件。
type Reservation struct {
 ok        bool
 lim       *Limiter
 tokens    int
 timeToAct time.Time
 // 这是预订时的 Limit，它以后可能会改变。
 limit Limit
}

// OK 返回 limiter 是否可以在最大等待时间内提供请求的令牌数。
// 如果 OK 为 false，Delay 返回 InfDuration，并且 Cancel 不执行任何操作。
func (r *Reservation) OK() bool {
 return r.ok
}

// Delay 是 DelayFrom(time.Now()) 的简写。
func (r *Reservation) Delay() time.Duration {
 return r.DelayFrom(time.Now())
}

// InfDuration 是 Delay 在 Reservation 不 OK 时返回的持续时间。
const InfDuration = time.Duration(math.MaxInt64)

// DelayFrom 返回预订持有者必须在执行保留操作之前等待的持续时间。
// 零持续时间意味着立即行动。
// InfDuration 意味着 limiter 不能在最大等待时间内授予此 Reservation 中请求的令牌。
func (r *Reservation) DelayFrom(t time.Time) time.Duration {
 if !r.ok {
  return InfDuration
 }
 delay := r.timeToAct.Sub(t)
 if delay < 0 {
  return 0
 }
 return delay
}

// Cancel 是 CancelAt(time.Now()) 的简写。
func (r *Reservation) Cancel() {
 r.CancelAt(time.Now())
}

// CancelAt 表示预订持有者不会执行保留的操作，并尽可能地撤销此 Reservation 对速率限制的影响，
// 考虑到可能已经进行了其他预订。
func (r *Reservation) CancelAt(t time.Time) {
 if !r.ok {
  return
 }

 r.lim.mu.Lock()
 defer r.lim.mu.Unlock()

 if r.lim.limit == Inf || r.tokens == 0 || r.timeToAct.Before(t) {
  return
 }

 // 计算要恢复的令牌
 // lim.lastEvent 和 r.timeToAct 之间的持续时间告诉我们预订后保留了多少令牌。这些令牌不应恢复。
 restoreTokens := float64(r.tokens) - r.limit.tokensFromDuration(r.lim.lastEvent.Sub(r.timeToAct))
 if restoreTokens <= 0 {
  return
 }
 // 将时间推进到现在
 t, tokens := r.lim.advance(t)
 // 计算新的令牌数量
 tokens += restoreTokens
 if burst := float64(r.lim.burst); tokens > burst {
  tokens = burst
 }
 // 更新状态
 r.lim.last = t
 r.lim.tokens = tokens
 if r.timeToAct == r.lim.lastEvent {
  prevEvent := r.timeToAct.Add(r.limit.durationFromTokens(float64(-r.tokens)))
  if !prevEvent.Before(t) {
   r.lim.lastEvent = prevEvent
  }
 }
}

// Reserve 是 ReserveN(time.Now(), 1) 的简写。
func (lim *Limiter) Reserve() *Reservation {
 return lim.ReserveN(time.Now(), 1)
}

// ReserveN 返回一个 Reservation，指示调用者必须在 n 个事件发生之前等待多长时间。
// Limiter 在允许未来事件时考虑此 Reservation。
// 返回的 Reservation 的 OK() 方法在 n 超过 Limiter 的突发大小时返回 false。
// 使用示例：
//
// r := lim.ReserveN(time.Now(), 1)
// if !r.OK() {
//   // 不允许行动！您是否记得将 lim.burst 设置为 > 0 ？
//   return
// }
// time.Sleep(r.Delay())
// Act()
//
// 如果您希望根据速率限制等待并放慢速度而不丢弃事件，请使用此方法。
```
