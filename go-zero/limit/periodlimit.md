### 代码加上注释版本

```go
package limit

import (
 "context"
 _ "embed"
 "errors"
 "strconv"
 "time"

 "github.com/zeromicro/go-zero/core/stores/redis"
)

const (
 // Unknown 表示未初始化的状态
 Unknown = iota
 // Allowed 表示允许的状态
 Allowed
 // HitQuota 表示此请求正好达到配额
 HitQuota
 // OverQuota 表示超过配额
 OverQuota

 internalOverQuota = 0
 internalAllowed   = 1
 internalHitQuota  = 2
)

var (
 // ErrUnknownCode 是一个表示未知状态码的错误
 ErrUnknownCode = errors.New("unknown status code")

 //go:embed periodscript.lua
 periodLuaScript string
 periodScript    = redis.NewScript(periodLuaScript)
)

type (
 // PeriodOption 定义了自定义 PeriodLimit 的方法
 PeriodOption func(l *PeriodLimit)

 // PeriodLimit 用于在一段时间内限制请求
 PeriodLimit struct {
  period     int           // 时间段长度
  quota      int           // 配额
  limitStore *redis.Redis  // Redis 存储
  keyPrefix  string        // 键前缀
  align      bool          // 是否对齐
 }
)

// NewPeriodLimit 返回一个带有给定参数的 PeriodLimit
func NewPeriodLimit(period, quota int, limitStore *redis.Redis, keyPrefix string,
 opts ...PeriodOption) *PeriodLimit {
 limiter := &PeriodLimit{
  period:     period,
  quota:      quota,
  limitStore: limitStore,
  keyPrefix:  keyPrefix,
 }

 for _, opt := range opts {
  opt(limiter)
 }

 return limiter
}

// Take 请求一个许可，返回许可状态
func (h *PeriodLimit) Take(key string) (int, error) {
 return h.TakeCtx(context.Background(), key)
}

// TakeCtx 带上下文请求一个许可，返回许可状态
func (h *PeriodLimit) TakeCtx(ctx context.Context, key string) (int, error) {
 resp, err := h.limitStore.ScriptRunCtx(ctx, periodScript, []string{h.keyPrefix + key}, []string{
  strconv.Itoa(h.quota),
  strconv.Itoa(h.calcExpireSeconds()),
 })
 if err != nil {
  return Unknown, err
 }

 code, ok := resp.(int64)
 if !ok {
  return Unknown, ErrUnknownCode
 }

 switch code {
 case internalOverQuota:
  return OverQuota, nil
 case internalAllowed:
  return Allowed, nil
 case internalHitQuota:
  return HitQuota, nil
 default:
  return Unknown, ErrUnknownCode
 }
}

// calcExpireSeconds 计算过期秒数
func (h *PeriodLimit) calcExpireSeconds() int {
 if h.align {
  now := time.Now()
  _, offset := now.Zone()
  unix := now.Unix() + int64(offset)
  return h.period - int(unix%int64(h.period))
 }

 return h.period
}

// Align 返回一个自定义 PeriodLimit 对齐的函数
// 例如，如果我们想限制用户每天发送 5 条短信验证消息，
// 我们需要与本地时区和一天的开始对齐。
func Align() PeriodOption {
 return func(l *PeriodLimit) {
  l.align = true
 }
}
```
