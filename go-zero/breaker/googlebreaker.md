### 代码加注释

```go
package breaker

import (
 "time"

 "github.com/zeromicro/go-zero/core/collection"
 "github.com/zeromicro/go-zero/core/mathx"
 "github.com/zeromicro/go-zero/core/syncx"
 "github.com/zeromicro/go-zero/core/timex"
)

const (
 // 每个桶的持续时间为250ms
 window            = time.Second * 10 // 窗口时间
 buckets           = 40               // 桶的数量
 forcePassDuration = time.Second      // 强制通过的持续时间
 k                 = 1.5              // 默认的k值
 minK              = 1.1              // 最小k值
 protection        = 5                // 保护阈值
)

// googleBreaker 是基于Google的NetflixBreaker模式实现的断路器
// 参考 <https://landing.google.com/sre/sre-book/chapters/handling-overload/> 中的Client-Side Throttling部分
type (
 googleBreaker struct {
  k        float64 // 调整因子
  stat     *collection.RollingWindow[int64, *bucket] // 滑动窗口统计
  proba*mathx.Proba // 概率计算器
  lastPass *syncx.AtomicDuration // 上次通过的时间
 }

 windowResult struct {
  accepts        int64 // 接受的请求数
  total          int64 // 总请求数
  failingBuckets int64 // 失败桶的数量
  workingBuckets int64 // 工作桶的数量
 }
)

// 创建一个新的googleBreaker实例
func newGoogleBreaker() *googleBreaker {
 bucketDuration := time.Duration(int64(window) / int64(buckets)) // 每个桶的持续时间, 250ms
 st := collection.NewRollingWindow[int64, *bucket](func()*bucket {
  return new(bucket)
 }, buckets, bucketDuration)
 return &googleBreaker{
  stat:     st,
  k:        k,
  proba:    mathx.NewProba(),
  lastPass: syncx.NewAtomicDuration(),
 }
}

// 判断是否接受请求
func (b *googleBreaker) accept() error {
 var w float64
 history := b.history()
 w = b.k - (b.k-minK)*float64(history.failingBuckets)/buckets
 weightedAccepts := mathx.AtLeast(w, minK) * float64(history.accepts)
 // 参考 <https://landing.google.com/sre/sre-book/chapters/handling-overload/#eq2101>
 // 为了更好的性能，不需要关心负比率
 dropRatio := (float64(history.total-protection) - weightedAccepts) / float64(history.total+1)
 if dropRatio <= 0 {
  return nil
 }

 lastPass := b.lastPass.Load()
 if lastPass > 0 && timex.Since(lastPass) > forcePassDuration {
  b.lastPass.Set(timex.Now())
  return nil
 }

 dropRatio *= float64(buckets-history.workingBuckets) / buckets

 if b.proba.TrueOnProba(dropRatio) {
  return ErrServiceUnavailable
 }

 b.lastPass.Set(timex.Now())

 return nil
}

// 允许请求并返回一个内部承诺
func (b *googleBreaker) allow() (internalPromise, error) {
 if err := b.accept(); err != nil {
  b.markDrop()
  return nil, err
 }

 return googlePromise{
  b: b,
 }, nil
}

// 执行请求，并根据结果标记成功或失败
func (b *googleBreaker) doReq(req func() error, fallback Fallback, acceptable Acceptable) error {
 if err := b.accept(); err != nil {
  b.markDrop()
  if fallback != nil {
   return fallback(err)
  }

  return err
 }

 var succ bool
 defer func() {
  // 如果req() panic，success为false，标记为失败
  if succ {
   b.markSuccess()
  } else {
   b.markFailure()
  }
 }()

 err := req()
 if acceptable(err) {
  succ = true
 }

 return err
}

// 标记请求被丢弃
func (b *googleBreaker) markDrop() {
 b.stat.Add(drop)
}

// 标记请求失败
func (b *googleBreaker) markFailure() {
 b.stat.Add(fail)
}

// 标记请求成功
func (b *googleBreaker) markSuccess() {
 b.stat.Add(success)
}

// 获取历史统计结果
func (b *googleBreaker) history() windowResult {
 var result windowResult

 b.stat.Reduce(func(b *bucket) {
  result.accepts += b.Success
  result.total += b.Sum
  if b.Failure > 0 {
   result.workingBuckets = 0
  } else if b.Success > 0 {
   result.workingBuckets++
  }
  if b.Success > 0 {
   result.failingBuckets = 0
  } else if b.Failure > 0 {
   result.failingBuckets++
  }
 })

 return result
}

type googlePromise struct {
 b *googleBreaker
}

// 接受请求
func (p googlePromise) Accept() {
 p.b.markSuccess()
}

// 拒绝请求
func (p googlePromise) Reject() {
 p.b.markFailure()
}
```
