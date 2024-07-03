### 代码加上注释版本

```go
package load

import (
 "sync/atomic"
 "time"

 "github.com/zeromicro/go-zero/core/logx"
 "github.com/zeromicro/go-zero/core/stat"
)

type (
 // SheddingStat 用于存储负载削减的统计数据。
 SheddingStat struct {
  name  string // 统计数据名称
  total int64  // 总请求数
  pass  int64  // 通过的请求数
  drop  int64  // 丢弃的请求数
 }

 // snapshot 用于存储统计数据的快照。
 snapshot struct {
  Total int64 // 总请求数
  Pass  int64 // 通过的请求数
  Drop  int64 // 丢弃的请求数
 }
)

// NewSheddingStat 返回一个新的 SheddingStat 实例，并启动一个 goroutine 来定期记录统计数据。
func NewSheddingStat(name string) *SheddingStat {
 st := &SheddingStat{
  name: name,
 }
 go st.run() // 启动一个 goroutine 来定期记录统计数据
 return st
}

// IncrementTotal 增加总请求数。
func (s *SheddingStat) IncrementTotal() {
 atomic.AddInt64(&s.total, 1)
}

// IncrementPass 增加通过的请求数。
func (s *SheddingStat) IncrementPass() {
 atomic.AddInt64(&s.pass, 1)
}

// IncrementDrop 增加丢弃的请求数。
func (s *SheddingStat) IncrementDrop() {
 atomic.AddInt64(&s.drop, 1)
}

// loop 方法定期从通道接收时间信号，并重置统计数据，记录日志。
func (s *SheddingStat) loop(c <-chan time.Time) {
 for range c {
  st := s.reset() // 重置统计数据并获取快照

  if !logEnabled.True() {
   continue
  }

  c := stat.CpuUsage() // 获取当前 CPU 使用率
  if st.Drop == 0 {
   logx.Statf("(%s) shedding_stat [1m], cpu: %d, total: %d, pass: %d, drop: %d",
    s.name, c, st.Total, st.Pass, st.Drop)
  } else {
   logx.Statf("(%s) shedding_stat_drop [1m], cpu: %d, total: %d, pass: %d, drop: %d",
    s.name, c, st.Total, st.Pass, st.Drop)
  }
 }
}

// reset 方法重置统计数据并返回当前的快照。
func (s *SheddingStat) reset() snapshot {
 return snapshot{
  Total: atomic.SwapInt64(&s.total, 0),
  Pass:  atomic.SwapInt64(&s.pass, 0),
  Drop:  atomic.SwapInt64(&s.drop, 0),
 }
}

// run 方法启动一个定时器，定期调用 loop 方法。
func (s *SheddingStat) run() {
 ticker := time.NewTicker(time.Minute) // 创建一个每分钟触发一次的定时器
 defer ticker.Stop()

 s.loop(ticker.C) // 启动 loop 方法，定期记录统计数据
}
```
