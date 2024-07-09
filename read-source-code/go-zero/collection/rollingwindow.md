```go
package collection

import (
 "sync"
 "time"

 "github.com/zeromicro/go-zero/core/mathx"
 "github.com/zeromicro/go-zero/core/timex"
)

type (
 // BucketInterface 定义了存储桶的接口。
 BucketInterface[T Numerical] interface {
  Add(v T)
  Reset()
 }

 // Numerical 限制了数值类型。
 Numerical = mathx.Numerical

 // RollingWindowOption 允许调用者自定义 RollingWindow。
 RollingWindowOption[T Numerical, B BucketInterface[T]] func(rollingWindow *RollingWindow[T, B])

 // RollingWindow 定义了一个滚动窗口，用于计算时间间隔内的桶中的事件。
 RollingWindow[T Numerical, B BucketInterface[T]] struct {
  lock          sync.RWMutex
  size          int
  win           *window[T, B]
  interval      time.Duration
  offset        int
  ignoreCurrent bool
  lastTime      time.Duration // 最后一个桶的开始时间
 }
)

// NewRollingWindow 返回一个具有指定大小和时间间隔的 RollingWindow，
// 使用 opts 来自定义 RollingWindow。
func NewRollingWindow[T Numerical, B BucketInterface[T]](newBucket func() B, size int,
 interval time.Duration, opts ...RollingWindowOption[T, B]) *RollingWindow[T, B] {
 if size < 1 {
  panic("size must be greater than 0")
 }

 w := &RollingWindow[T, B]{
  size:     size,
  win:      newWindow[T, B](newBucket, size),
  interval: interval,
  lastTime: timex.Now(),
 }
 for _, opt := range opts {
  opt(w)
 }
 return w
}

// Add 将值添加到当前桶中。
func (rw *RollingWindow[T, B]) Add(v T) {
 rw.lock.Lock()
 defer rw.lock.Unlock()
 rw.updateOffset()
 rw.win.add(rw.offset, v)
}

// Reduce 在所有桶上运行 fn，如果设置了 ignoreCurrent，则忽略当前桶。
func (rw *RollingWindow[T, B]) Reduce(fn func(b B)) {
 rw.lock.RLock()
 defer rw.lock.RUnlock()

 var diff int
 span := rw.span()
 // 忽略当前桶，因为数据不完整
 if span == 0 && rw.ignoreCurrent {
  diff = rw.size - 1
 } else {
  diff = rw.size - span
 }
 if diff > 0 {
  offset := (rw.offset + span + 1) % rw.size
  rw.win.reduce(offset, diff, fn)
 }
}

func (rw *RollingWindow[T, B]) span() int {
 offset := int(timex.Since(rw.lastTime) / rw.interval)
 if 0 <= offset && offset < rw.size {
  return offset
 }

 return rw.size
}

func (rw *RollingWindow[T, B]) updateOffset() {
 span := rw.span()
 if span <= 0 {
  return
 }

 offset := rw.offset
 // 重置过期的桶
 for i := 0; i < span; i++ {
  rw.win.resetBucket((offset + i + 1) % rw.size)
 }

 rw.offset = (offset + span) % rw.size
 now := timex.Now()
 // 对齐到时间间隔边界
 rw.lastTime = now - (now-rw.lastTime)%rw.interval
}

// Bucket 定义了存储桶，包含总和和添加次数。
type Bucket[T Numerical] struct {
 Sum   T
 Count int64
}

func (b *Bucket[T]) Add(v T) {
 b.Sum += v
 b.Count++
}

func (b *Bucket[T]) Reset() {
 b.Sum = 0
 b.Count = 0
}

type window[T Numerical, B BucketInterface[T]] struct {
 buckets []B
 size    int
}

func newWindow[T Numerical, B BucketInterface[T]](newBucket func() B, size int) *window[T, B] {
 buckets := make([]B, size)
 for i := 0; i < size; i++ {
  buckets[i] = newBucket()
 }
 return &window[T, B]{
  buckets: buckets,
  size:    size,
 }
}

func (w *window[T, B]) add(offset int, v T) {
 w.buckets[offset%w.size].Add(v)
}

func (w *window[T, B]) reduce(start, count int, fn func(b B)) {
 for i := 0; i < count; i++ {
  fn(w.buckets[(start+i)%w.size])
 }
}

func (w *window[T, B]) resetBucket(offset int) {
 w.buckets[offset%w.size].Reset()
}

// IgnoreCurrentBucket 让 Reduce 调用忽略当前桶。
func IgnoreCurrentBucket[T Numerical, B BucketInterface[T]]() RollingWindowOption[T, B] {
 return func(w *RollingWindow[T, B]) {
  w.ignoreCurrent = true
 }
}
```
