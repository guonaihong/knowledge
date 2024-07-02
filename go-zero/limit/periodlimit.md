### 固定窗口限流算法

#### 一、解释

固定窗口限流算法（Fixed Window Rate Limiting Algorithm）是一种最简单的限流算法，其原理是在固定时间窗口(单位时间)内限制请求的数量。该算法将时间分成固定的窗口，并在每个窗口内限制请求的数量。具体来说，算法将请求按照时间顺序放入时间窗口中，并计算该时间窗口内的请求数量，如果请求数量超出了限制，则拒绝该请求。

* go-zero固定窗口限流器的实现

什么是基于固定窗口计数的限流式，比如一个时间片(假设1s)，最大允许的请求是100，超过这个值，直接报错。

在工程实现上一般要追求容错性和运维方面，一般会把实现放在redis。比如go-zero

go-zero的计算是保存在redis里面的，为了保证原子性，这块的逻辑是lua脚本实现的
<https://mp.weixin.qq.com/s/CTemkZ2aKPCPTuQiDJri0Q>

#### 二、lua脚本实现原理

下面的lua脚本，主要干了2件事[periodlimit.lua](./periodlimit_lua.md)

* 通过lua的incrby原子计计数
* 如果第一次访问就加过期时间，主要是为了清空这个窗口
* 返回访问数是否到达限制(1 未到达限制， 2 已达到限制， 0 超出限制)

#### 三、该算法的缺点

假设每个窗口的时间是1s，最大请求数是100, 第一个窗口后0.5s访问了 100, 第二个窗口的前0.5s访问了100，那这段时间实际访问了200，达到限流数的2倍。

#### 四、代码加上注释版本

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
