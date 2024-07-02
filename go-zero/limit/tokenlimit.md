### 代码加上注释版本

```go
package limit

import (
 "context"
 _ "embed"
 "errors"
 "fmt"
 "strconv"
 "sync"
 "sync/atomic"
 "time"

 "github.com/zeromicro/go-zero/core/errorx"
 "github.com/zeromicro/go-zero/core/logx"
 "github.com/zeromicro/go-zero/core/stores/redis"
 xrate "golang.org/x/time/rate"
)

const (
 tokenFormat     = "{%s}.tokens"     // 令牌桶键格式
 timestampFormat = "{%s}.ts"         // 时间戳键格式
 pingInterval    = time.Millisecond * 100 // Redis 健康检查间隔
)

var (
 //go:embed tokenscript.lua
 tokenLuaScript string
 tokenScript    = redis.NewScript(tokenLuaScript)
)

// TokenLimiter 控制事件在一秒内的发生频率。
type TokenLimiter struct {
 rate           int           // 每秒允许的事件数
 burst          int           // 允许的突发事件数
 store          *redis.Redis  // Redis 存储实例
 tokenKey       string        // 令牌桶键
 timestampKey   string        // 时间戳键
 rescueLock     sync.Mutex    // 救援锁
 redisAlive     uint32        // Redis 是否存活标志
 monitorStarted bool          // 监控是否已启动标志
 rescueLimiter  *xrate.Limiter // 本地救援限流器
}

// NewTokenLimiter 返回一个新的 TokenLimiter，允许每秒最多 rate 个事件，突发最多 burst 个事件。
func NewTokenLimiter(rate, burst int, store *redis.Redis, key string) *TokenLimiter {
 tokenKey := fmt.Sprintf(tokenFormat, key)
 timestampKey := fmt.Sprintf(timestampFormat, key)

 return &TokenLimiter{
  rate:          rate,
  burst:         burst,
  store:         store,
  tokenKey:      tokenKey,
  timestampKey:  timestampKey,
  redisAlive:    1,
  rescueLimiter: xrate.NewLimiter(xrate.Every(time.Second/time.Duration(rate)), burst),
 }
}

// Allow 是 AllowN(time.Now(), 1) 的简写。
func (lim *TokenLimiter) Allow() bool {
 return lim.AllowN(time.Now(), 1)
}

// AllowCtx 是 AllowNCtx(ctx, time.Now(), 1) 的简写，带有传入的上下文。
func (lim *TokenLimiter) AllowCtx(ctx context.Context) bool {
 return lim.AllowNCtx(ctx, time.Now(), 1)
}

// AllowN 报告在时间 now 是否可以发生 n 个事件。
// 如果你想丢弃/跳过超过速率的事件，使用此方法。
// 否则，使用 Reserve 或 Wait。
func (lim *TokenLimiter) AllowN(now time.Time, n int) bool {
 return lim.reserveN(context.Background(), now, n)
}

// AllowNCtx 报告在时间 now 是否可以发生 n 个事件，带有传入的上下文。
// 如果你想丢弃/跳过超过速率的事件，使用此方法。
// 否则，使用 Reserve 或 Wait。
func (lim *TokenLimiter) AllowNCtx(ctx context.Context, now time.Time, n int) bool {
 return lim.reserveN(ctx, now, n)
}

// reserveN 在时间 now 是否可以发生 n 个事件，带有传入的上下文。
func (lim *TokenLimiter) reserveN(ctx context.Context, now time.Time, n int) bool {
 if atomic.LoadUint32(&lim.redisAlive) == 0 {
  return lim.rescueLimiter.AllowN(now, n)
 }

 resp, err := lim.store.ScriptRunCtx(ctx,
  tokenScript,
  []string{
   lim.tokenKey,
   lim.timestampKey,
  },
  []string{
   strconv.Itoa(lim.rate),
   strconv.Itoa(lim.burst),
   strconv.FormatInt(now.Unix(), 10),
   strconv.Itoa(n),
  })
 // redis allowed == false
 // Lua boolean false -> r Nil bulk reply
 if errors.Is(err, redis.Nil) {
  return false
 }
 if errorx.In(err, context.DeadlineExceeded, context.Canceled) {
  logx.Errorf("fail to use rate limiter: %s", err)
  return false
 }
 if err != nil {
  logx.Errorf("fail to use rate limiter: %s, use in-process limiter for rescue", err)
  lim.startMonitor()
  return lim.rescueLimiter.AllowN(now, n)
 }

 code, ok := resp.(int64)
 if !ok {
  logx.Errorf("fail to eval redis script: %v, use in-process limiter for rescue", resp)
  lim.startMonitor()
  return lim.rescueLimiter.AllowN(now, n)
 }

 // redis allowed == true
 // Lua boolean true -> r integer reply with value of 1
 return code == 1
}

// startMonitor 启动 Redis 健康检查监控。
func (lim *TokenLimiter) startMonitor() {
 lim.rescueLock.Lock()
 defer lim.rescueLock.Unlock()

 if lim.monitorStarted {
  return
 }

 lim.monitorStarted = true
 atomic.StoreUint32(&lim.redisAlive, 0)

 go lim.waitForRedis()
}

// waitForRedis 等待 Redis 恢复。
func (lim *TokenLimiter) waitForRedis() {
 ticker := time.NewTicker(pingInterval)
 defer func() {
  ticker.Stop()
  lim.rescueLock.Lock()
  lim.monitorStarted = false
  lim.rescueLock.Unlock()
 }()

 for range ticker.C {
  if lim.store.Ping() {
   atomic.StoreUint32(&lim.redisAlive, 1)
   return
  }
 }
}
```
