### 文字解释

这些代码片段是 adaptiveShedder 结构体的方法，用于实现自适应的负载削减（load shedding）。下面是对每个方法的详细解释：

#### 1.addFlying

这个方法用于更新当前正在处理的请求数（flying）和平均正在处理的请求数（avgFlying）。

delta 参数表示请求数的增量，可以是正数（增加请求）或负数（减少请求）。

当 delta 为负数时，表示请求完成，此时更新 avgFlying，使用移动平均算法使其变化更加平滑。

#### 2.highThru

这个方法用于判断当前系统的吞吐量是否过高。

它比较平均正在处理的请求数（avgFlying）和允许的最大并发请求数（maxFlight），以及当前正在处理的请求数（flying）是否超过 maxFlight。

如果两者都超过，则认为系统吞吐量过高。

#### 3.maxFlight

这个方法计算允许的最大并发请求数。

它基于系统的最大吞吐量（QPS）和最小响应时间（RT）来计算。

maxPass 方法返回最大通过请求数，minRt 方法返回最小响应时间，windowScale 是一个调整因子。

#### 4.maxPass

这个方法返回滑动窗口中通过请求数的最大值。

它遍历 passCounter 中的所有桶，找到通过请求数的最大值。

#### 5.minRt

这个方法返回滑动窗口中最小响应时间的平均值。

如果前一个窗口没有请求，返回默认的最小响应时间（defaultMinRt）。

#### 6.overloadFactor

这个方法计算过载因子，用于调整允许的最大并发请求数。

它基于当前 CPU 使用率和设定的 CPU 阈值来计算。

确保至少接受 10% 的可接受请求，即使 CPU 高度过载。

#### 7.shouldDrop

这个方法判断是否应该丢弃当前请求。

如果系统过载或仍然处于热状态（最近丢弃过请求），并且吞吐量过高，则丢弃请求。

丢弃请求时，记录日志并报告状态。

#### 8.stillHot

这个方法判断系统是否仍然处于热状态。

如果最近丢弃过请求，并且过载时间在冷却时间内，则认为系统仍然处于热状态。

#### 9.systemOverloaded

这个方法判断系统是否过载。

如果当前 CPU 使用率超过设定的 CPU 阈值，则认为系统过载，并记录过载时间。

WithBuckets、WithCpuThreshold、WithWindow：

这些方法用于自定义 Shedder 的配置选项，如桶的数量、CPU 阈值和时间窗口。

#### 10.promise

这个结构体用于表示一个请求的承诺。

Fail 方法在请求失败时调用，减少正在处理的请求数。

Pass 方法在请求成功时调用，减少正在处理的请求数，并记录响应时间和通过的请求数。

这些方法共同协作，实现了自适应的负载削减机制，根据系统的实时状态动态调整允许的并发请求数，以避免系统过载。

### 代码加注释版本

```go
package load

import (
 "errors"
 "fmt"
 "math"
 "sync/atomic"
 "time"

 "github.com/zeromicro/go-zero/core/collection"
 "github.com/zeromicro/go-zero/core/logx"
 "github.com/zeromicro/go-zero/core/mathx"
 "github.com/zeromicro/go-zero/core/stat"
 "github.com/zeromicro/go-zero/core/syncx"
 "github.com/zeromicro/go-zero/core/timex"
)

const (
 defaultBuckets = 50 // 默认桶数量
 defaultWindow  = time.Second * 5 // 默认时间窗口
 // 使用1000m表示法，900m类似于90%，保持为变量以便单元测试
 defaultCpuThreshold = 900
 defaultMinRt        = float64(time.Second / time.Millisecond) // 默认最小响应时间
 // 移动平均超参数beta，用于实时计算请求
 flyingBeta               = 0.9
 coolOffDuration          = time.Second // 冷却时间
 cpuMax                   = 1000 // 毫核
 millisecondsPerSecond    = 1000
 overloadFactorLowerBound = 0.1 // 过载因子下限
)

var (
 // ErrServiceOverloaded 当服务过载时由Shedder.Allow返回
 ErrServiceOverloaded = errors.New("service overloaded")

 // 默认启用
 enabled = syncx.ForAtomicBool(true)
 // 默认启用日志
 logEnabled = syncx.ForAtomicBool(true)
 // 单元测试变量
 systemOverloadChecker = func(cpuThreshold int64) bool {
  return stat.CpuUsage() >= cpuThreshold
 }
)

type (
 // Promise接口由Shedder.Allow返回，让调用者告知请求处理是否成功
 Promise interface {
  // Pass 告知调用成功
  Pass()
  // Fail 告知调用失败
  Fail()
 }

 // Shedder接口包装了Allow方法
 Shedder interface {
  // Allow 如果允许则返回Promise，否则返回ErrServiceOverloaded
  Allow() (Promise, error)
 }

 // ShedderOption允许调用者自定义Shedder
 ShedderOption func(opts *shedderOptions)

 shedderOptions struct {
  window       time.Duration
  buckets      int
  cpuThreshold int64
 }

 adaptiveShedder struct {
  cpuThreshold    int64
  windowScale     float64
  flying          int64
  avgFlying       float64
  avgFlyingLock   syncx.SpinLock
  overloadTime    *syncx.AtomicDuration
  droppedRecently *syncx.AtomicBool
  passCounter     *collection.RollingWindow[int64, *collection.Bucket[int64]]
  rtCounter       *collection.RollingWindow[int64, *collection.Bucket[int64]]
 }
)

// Disable 允许调用者禁用负载 shedding
func Disable() {
 enabled.Set(false)
}

// DisableLog 禁用负载 shedding 的统计日志
func DisableLog() {
 logEnabled.Set(false)
}

// NewAdaptiveShedder 返回一个自适应的 shedder
// opts 可以用来定制 Shedder
func NewAdaptiveShedder(opts ...ShedderOption) Shedder {
 if !enabled.True() {
  return newNopShedder()
 }

 options := shedderOptions{
  window:       defaultWindow,
  buckets:      defaultBuckets,
  cpuThreshold: defaultCpuThreshold,
 }
 for _, opt := range opts {
  opt(&options)
 }
 bucketDuration := options.window / time.Duration(options.buckets)
 newBucket := func() *collection.Bucket[int64] {
  return new(collection.Bucket[int64])
 }
 return &adaptiveShedder{
  cpuThreshold:    options.cpuThreshold,
  windowScale:     float64(time.Second) / float64(bucketDuration) / millisecondsPerSecond,
  overloadTime:    syncx.NewAtomicDuration(),
  droppedRecently: syncx.NewAtomicBool(),
  passCounter:     collection.NewRollingWindow[int64, *collection.Bucket[int64]](newBucket, options.buckets, bucketDuration, collection.IgnoreCurrentBucket[int64, *collection.Bucket[int64]]()),
  rtCounter:       collection.NewRollingWindow[int64, *collection.Bucket[int64]](newBucket, options.buckets, bucketDuration, collection.IgnoreCurrentBucket[int64, *collection.Bucket[int64]]()),
 }
}

// Allow 实现 Shedder.Allow
func (as *adaptiveShedder) Allow() (Promise, error) {
 if as.shouldDrop() {
  as.droppedRecently.Set(true)

  return nil, ErrServiceOverloaded
 }

 as.addFlying(1)

 return &promise{
  start:   timex.Now(),
  shedder: as,
 }, nil
}

func (as *adaptiveShedder) addFlying(delta int64) {
 flying := atomic.AddInt64(&as.flying, delta)
 // 当请求完成时更新 avgFlying
 // 这种策略使得 avgFlying 相对于 flying 有一定的滞后，更加平滑
 // 当 flying 请求快速增加时，avgFlying 增加较慢，接受更多请求
 // 当 flying 请求快速下降时，avgFlying 下降较慢，接受更少请求
 // 这使得服务尽可能多地处理请求
 if delta < 0 {
  as.avgFlyingLock.Lock()
  as.avgFlying = as.avgFlying*flyingBeta + float64(flying)*(1-flyingBeta)
  as.avgFlyingLock.Unlock()
 }
}

func (as *adaptiveShedder) highThru() bool {
 as.avgFlyingLock.Lock()
 avgFlying := as.avgFlying
 as.avgFlyingLock.Unlock()
 maxFlight := as.maxFlight() * as.overloadFactor()
 return avgFlying > maxFlight && float64(atomic.LoadInt64(&as.flying)) > maxFlight
}

// maxFlight 计算最大允许的请求
func (as *adaptiveShedder) maxFlight() float64 {
 // windows = 每秒桶数
 // maxQPS = maxPASS * windows
 // minRT = 最小平均响应时间（毫秒）
 // allowedFlying = maxQPS * minRT / 每秒毫秒数
 maxFlight := float64(as.maxPass()) * as.minRt() * as.windowScale
 return mathx.AtLeast(maxFlight, 1)
}

func (as *adaptiveShedder) maxPass() int64 {
 var result int64 = 1

 as.passCounter.Reduce(func(b *collection.Bucket[int64]) {
  if b.Sum > result {
   result = b.Sum
  }
 })

 return result
}

func (as *adaptiveShedder) minRt() float64 {
 // 如果前一个窗口没有请求，返回 defaultMinRt
 // 这是一个合理的大值，以避免丢弃请求
 result := defaultMinRt

 as.rtCounter.Reduce(func(b *collection.Bucket[int64]) {
  if b.Count <= 0 {
   return
  }

  avg := math.Round(float64(b.Sum) / float64(b.Count))
  if avg < result {
   result = avg
  }
 })

 return result
}

// overloadFactor 计算 CPU 超负载因子
func (as *adaptiveShedder) overloadFactor() float64 {
 // as.cpuThreshold 必须小于 cpuMax
 factor := (cpuMax - float64(stat.CpuUsage())) / (cpuMax - float64(as.cpuThreshold))
 // 至少接受 10% 的可接受请求，即使 CPU 高度过载
 return mathx.Between(factor, overloadFactorLowerBound, 1)
}

func (as *adaptiveShedder) shouldDrop() bool {
 if as.systemOverloaded() || as.stillHot() {
  if as.highThru() {
   flying := atomic.LoadInt64(&as.flying)
   as.avgFlyingLock.Lock()
   avgFlying := as.avgFlying
   as.avgFlyingLock.Unlock()
   msg := fmt.Sprintf(
    "dropreq, cpu: %d, maxPass: %d, minRt: %.2f, hot: %t, flying: %d, avgFlying: %.2f",
    stat.CpuUsage(), as.maxPass(), as.minRt(), as.stillHot(), flying, avgFlying)
   logx.Error(msg)
   stat.Report(msg)
   return true
  }
 }

 return false
}

func (as *adaptiveShedder) stillHot() bool {
 if !as.droppedRecently.True() {
  return false
 }

 overloadTime := as.overloadTime.Load()
 if overloadTime == 0 {
  return false
 }

 if timex.Since(overloadTime) < coolOffDuration {
  return true
 }

 as.droppedRecently.Set(false)
 return false
}

func (as *adaptiveShedder) systemOverloaded() bool {
 if !systemOverloadChecker(as.cpuThreshold) {
  return false
 }

 as.overloadTime.Set(timex.Now())
 return true
}

// WithBuckets 使用给定的桶数量自定义 Shedder
func WithBuckets(buckets int) ShedderOption {
 return func(opts *shedderOptions) {
  opts.buckets = buckets
 }
}

// WithCpuThreshold 使用给定的 CPU 阈值自定义 Shedder
func WithCpuThreshold(threshold int64) ShedderOption {
 return func(opts *shedderOptions) {
  opts.cpuThreshold = threshold
 }
}

// WithWindow 使用给定的时间窗口自定义 Shedder
func WithWindow(window time.Duration) ShedderOption {
 return func(opts *shedderOptions) {
  opts.window = window
 }
}

type promise struct {
 start   time.Duration
 shedder *adaptiveShedder
}

func (p *promise) Fail() {
 p.shedder.addFlying(-1)
}

func (p *promise) Pass() {
 rt := float64(timex.Since(p.start)) / float64(time.Millisecond)
 p.shedder.addFlying(-1)
 p.shedder.rtCounter.Add(int64(math.Ceil(rt)))
 p.shedder.passCounter.Add(1)
}
```
