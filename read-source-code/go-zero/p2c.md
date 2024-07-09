### 分析

* 使用指数加权移动平均算法(EWMA)算法，实现负载均衡算法
* 其中EWMA需要用到的系统，通过y=e^-x的公式计算

### 源代码加注释版本

```go
package p2c

import (
 "fmt"
 "math"
 "math/rand"
 "strings"
 "sync"
 "sync/atomic"
 "time"

 "github.com/zeromicro/go-zero/core/logx"
 "github.com/zeromicro/go-zero/core/syncx"
 "github.com/zeromicro/go-zero/core/timex"
 "github.com/zeromicro/go-zero/zrpc/internal/codes"
 "google.golang.org/grpc/balancer"
 "google.golang.org/grpc/balancer/base"
 "google.golang.org/grpc/resolver"
)

const (
 // 定义p2c balancer的名称
 Name = "p2c_ewma"

 // 衰减时间，用于计算EWMA
 decayTime = int64(time.Second * 10)
 // 强制选择的时间间隔
 forcePick = int64(time.Second)
 // 初始成功次数
 initSuccess = 1000
 // 触发限流的成功次数
 throttleSuccess = initSuccess / 2
 // 惩罚值，用于负载计算
 penalty = int64(math.MaxInt32)
 // 选择尝试次数
 pickTimes = 3
 // 日志统计间隔
 logInterval = time.Minute
)

// 空的选择结果
var emptyPickResult balancer.PickResult

func init() {
 // 注册p2c balancer构建器
 balancer.Register(newBuilder())
}

// p2cPickerBuilder用于构建p2c选择器
type p2cPickerBuilder struct{}

// Build方法根据给定的连接信息构建选择器
func (b *p2cPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
 readySCs := info.ReadySCs
 if len(readySCs) == 0 {
  return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
 }

 var conns []*subConn
 for conn, connInfo := range readySCs {
  conns = append(conns, &subConn{
   addr:    connInfo.Address,
   conn:    conn,
   success: initSuccess,
  })
 }

 return &p2cPicker{
  conns: conns,
  r:     rand.New(rand.NewSource(time.Now().UnixNano())),
  stamp: syncx.NewAtomicDuration(),
 }
}

// 创建新的p2c balancer构建器
func newBuilder() balancer.Builder {
 return base.NewBalancerBuilder(Name, new(p2cPickerBuilder), base.Config{HealthCheck: true})
}

// p2cPicker结构体，用于实现负载均衡选择逻辑
type p2cPicker struct {
 conns []*subConn
 r     *rand.Rand
 stamp *syncx.AtomicDuration
 lock  sync.Mutex
}

// Pick方法用于选择一个连接进行处理
func (p *p2cPicker) Pick(_ balancer.PickInfo) (balancer.PickResult, error) {
 p.lock.Lock()
 defer p.lock.Unlock()

 var chosen *subConn
 switch len(p.conns) {
 case 0:
  return emptyPickResult, balancer.ErrNoSubConnAvailable
 case 1:
  chosen = p.choose(p.conns[0], nil)
 case 2:
  chosen = p.choose(p.conns[0], p.conns[1])
 default:
  var node1, node2 *subConn
  for i := 0; i < pickTimes; i++ {
   a := p.r.Intn(len(p.conns))
   b := p.r.Intn(len(p.conns) - 1)
   if b >= a {
    b++
   }
   node1 = p.conns[a]
   node2 = p.conns[b]
   if node1.healthy() && node2.healthy() {
    break
   }
  }

  chosen = p.choose(node1, node2)
 }

 atomic.AddInt64(&chosen.inflight, 1)
 atomic.AddInt64(&chosen.requests, 1)

 return balancer.PickResult{
  SubConn: chosen.conn,
  Done:    p.buildDoneFunc(chosen),
 }, nil
}

// 构建Done函数，用于处理请求完成后的逻辑
func (p *p2cPicker) buildDoneFunc(c *subConn) func(info balancer.DoneInfo) {
 start := int64(timex.Now())
 return func(info balancer.DoneInfo) {
  atomic.AddInt64(&c.inflight, -1)
  now := timex.Now()
  last := atomic.SwapInt64(&c.last, int64(now))
  td := int64(now) - last
  if td < 0 {
   td = 0
  }

  // *
  //  *
  //   *
  //    *
  //     *
  //      *
  //       *
  //        *
  //         *
  //          *
  //           **
  //             *
  //              ***
  //                 **
  //                   ****
  //                       ******
  //                             *********************
  //                                                  *
  //
  w := math.Exp(float64(-td) / float64(decayTime))
  lag := int64(now) - start
  if lag < 0 {
   lag = 0
  }
  olag := atomic.LoadUint64(&c.lag)
  if olag == 0 {
   w = 0
  }
  // w越大，历史数据影响越大，越小，现在的数据影响就越大
  atomic.StoreUint64(&c.lag, uint64(float64(olag)*w+float64(lag)*(1-w)))
  success := initSuccess
  if info.Err != nil && !codes.Acceptable(info.Err) {
   success = 0
  }
  osucc := atomic.LoadUint64(&c.success)
  atomic.StoreUint64(&c.success, uint64(float64(osucc)*w+float64(success)*(1-w)))

  stamp := p.stamp.Load()
  if now-stamp >= logInterval {
   if p.stamp.CompareAndSwap(stamp, now) {
    p.logStats()
   }
  }
 }
}

// 选择函数，用于从两个连接中选择一个
func (p *p2cPicker) choose(c1, c2 *subConn) *subConn {
 start := int64(timex.Now())
 if c2 == nil {
  atomic.StoreInt64(&c1.pick, start)
  return c1
 }

 if c1.load() > c2.load() {
  c1, c2 = c2, c1
 }

 pick := atomic.LoadInt64(&c2.pick)
 if start-pick > forcePick && atomic.CompareAndSwapInt64(&c2.pick, pick, start) {
  return c2
 }

 atomic.StoreInt64(&c1.pick, start)
 return c1
}

// 记录统计信息
func (p *p2cPicker) logStats() {
 var stats []string

 p.lock.Lock()
 defer p.lock.Unlock()

 for _, conn := range p.conns {
  stats = append(stats, fmt.Sprintf("conn: %s, load: %d, reqs: %d",
   conn.addr.Addr, conn.load(), atomic.SwapInt64(&conn.requests, 0)))
 }

 logx.Statf("p2c - %s", strings.Join(stats, "; "))
}

// subConn结构体，表示一个子连接
type subConn struct {
 lag      uint64
 inflight int64
 success  uint64
 requests int64
 last     int64
 pick     int64
 addr     resolver.Address
 conn     balancer.SubConn
}

// 检查连接是否健康
func (c *subConn) healthy() bool {
 return atomic.LoadUint64(&c.success) > throttleSuccess
}

// 计算连接的负载
func (c *subConn) load() int64 {
 // 加一避免乘以零
 lag := int64(math.Sqrt(float64(atomic.LoadUint64(&c.lag) + 1)))
 load := lag * (atomic.LoadInt64(&c.inflight) + 1)
 if load == 0 {
  return penalty
 }

 return load
}
```

### 参加资料

<https://exceting.github.io/2020/08/13/%E8%B4%9F%E8%BD%BD%E5%9D%87%E8%A1%A1-P2C%E7%AE%97%E6%B3%95/>
