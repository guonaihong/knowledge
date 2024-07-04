
### 场景

在日常开发中，经常遇到不好复现的问题。如果能拿到栈信息。
这时候可以使用 `go tool pprof` 命令来分析问题。难题是如何保存现场。
go-zero里面默认集成了，根据信号dump调用栈的代码

* SIGUSR1: dump goroutines
* SIGUSR2: dump profile, mem, mutex, block, trace, threadcreate

```go
 prof.startCpuProfile()
 prof.startMemProfile()
 prof.startMutexProfile()
 prof.startBlockProfile()
 prof.startTraceProfile()
 prof.startThreadCreateProfile()
```

如果不用go-zero框架，也可以写个http server，根据接口来动态关启或者关闭profile, 或者结合监控系统，来打开或者关闭profile。

### 代码加上注释

```go
//go:build linux || darwin

// 包 proc 提供了对 Go 应用程序进行性能分析的功能。
package proc

import (
 "fmt"
 "os"
 "os/signal"
 "path"
 "runtime"
 "runtime/pprof"
 "runtime/trace"
 "sync/atomic"
 "syscall"
 "time"

 "github.com/zeromicro/go-zero/core/logx"
)

// DefaultMemProfileRate 是默认的内存分析速率。
// 参考 http://golang.org/pkg/runtime/#pkg-variables
const DefaultMemProfileRate = 4096

// started 是一个标志，指示是否有一个分析会话正在进行。
var started uint32

// Profile 结构体表示一个活动的分析会话。
type Profile struct {
 // closers 保存了每个分析结束后运行的清理函数
 closers []func()

 // stopped 记录是否已经调用了 profile.Stop 方法
 stopped uint32
}

// close 方法执行所有清理函数。
func (p *Profile) close() {
 for _, closer := range p.closers {
  closer()
 }
}

// startBlockProfile 方法启动阻塞分析。
func (p *Profile) startBlockProfile() {
 fn := createDumpFile("block")
 f, err := os.Create(fn)
 if err != nil {
  logx.Errorf("profile: 无法创建阻塞分析文件 %q: %v", fn, err)
  return
 }

 runtime.SetBlockProfileRate(1)
 logx.Infof("profile: 阻塞分析已启用, %s", fn)
 p.closers = append(p.closers, func() {
  pprof.Lookup("block").WriteTo(f, 0)
  f.Close()
  runtime.SetBlockProfileRate(0)
  logx.Infof("profile: 阻塞分析已禁用, %s", fn)
 })
}

// startCpuProfile 方法启动 CPU 分析。
func (p *Profile) startCpuProfile() {
 fn := createDumpFile("cpu")
 f, err := os.Create(fn)
 if err != nil {
  logx.Errorf("profile: 无法创建 CPU 分析文件 %q: %v", fn, err)
  return
 }

 logx.Infof("profile: CPU 分析已启用, %s", fn)
 pprof.StartCPUProfile(f)
 p.closers = append(p.closers, func() {
  pprof.StopCPUProfile()
  f.Close()
  logx.Infof("profile: CPU 分析已禁用, %s", fn)
 })
}

// startMemProfile 方法启动内存分析。
func (p *Profile) startMemProfile() {
 fn := createDumpFile("mem")
 f, err := os.Create(fn)
 if err != nil {
  logx.Errorf("profile: 无法创建内存分析文件 %q: %v", fn, err)
  return
 }

 old := runtime.MemProfileRate
 runtime.MemProfileRate = DefaultMemProfileRate
 logx.Infof("profile: 内存分析已启用 (速率 %d), %s", runtime.MemProfileRate, fn)
 p.closers = append(p.closers, func() {
  pprof.Lookup("heap").WriteTo(f, 0)
  f.Close()
  runtime.MemProfileRate = old
  logx.Infof("profile: 内存分析已禁用, %s", fn)
 })
}

// startMutexProfile 方法启动互斥锁分析。
func (p *Profile) startMutexProfile() {
 fn := createDumpFile("mutex")
 f, err := os.Create(fn)
 if err != nil {
  logx.Errorf("profile: 无法创建互斥锁分析文件 %q: %v", fn, err)
  return
 }

 runtime.SetMutexProfileFraction(1)
 logx.Infof("profile: 互斥锁分析已启用, %s", fn)
 p.closers = append(p.closers, func() {
  if mp := pprof.Lookup("mutex"); mp != nil {
   mp.WriteTo(f, 0)
  }
  f.Close()
  runtime.SetMutexProfileFraction(0)
  logx.Infof("profile: 互斥锁分析已禁用, %s", fn)
 })
}

// startThreadCreateProfile 方法启动线程创建分析。
func (p *Profile) startThreadCreateProfile() {
 fn := createDumpFile("threadcreate")
 f, err := os.Create(fn)
 if err != nil {
  logx.Errorf("profile: 无法创建线程创建分析文件 %q: %v", fn, err)
  return
 }

 logx.Infof("profile: 线程创建分析已启用, %s", fn)
 p.closers = append(p.closers, func() {
  if mp := pprof.Lookup("threadcreate"); mp != nil {
   mp.WriteTo(f, 0)
  }
  f.Close()
  logx.Infof("profile: 线程创建分析已禁用, %s", fn)
 })
}

// startTraceProfile 方法启动跟踪分析。
func (p *Profile) startTraceProfile() {
 fn := createDumpFile("trace")
 f, err := os.Create(fn)
 if err != nil {
  logx.Errorf("profile: 无法创建跟踪输出文件 %q: %v", fn, err)
  return
 }

 if err := trace.Start(f); err != nil {
  logx.Errorf("profile: 无法启动跟踪: %v", err)
  return
 }

 logx.Infof("profile: 跟踪已启用, %s", fn)
 p.closers = append(p.closers, func() {
  trace.Stop()
  logx.Infof("profile: 跟踪已禁用, %s", fn)
 })
}

// Stop 方法停止分析会话并刷新任何未写入的数据。
func (p *Profile) Stop() {
 if !atomic.CompareAndSwapUint32(&p.stopped, 0, 1) {
  // 已经有人调用了 close
  return
 }
 p.close()
 atomic.StoreUint32(&started, 0)
}

// StartProfile 启动一个新的分析会话。
// 调用者应该在返回的值上调用 Stop 方法以干净地停止分析。
func StartProfile() Stopper {
 if !atomic.CompareAndSwapUint32(&started, 0, 1) {
  logx.Error("profile: Start() 已经被调用")
  return noopStopper
 }

 var prof Profile
 prof.startCpuProfile()
 prof.startMemProfile()
 prof.startMutexProfile()
 prof.startBlockProfile()
 prof.startTraceProfile()
 prof.startThreadCreateProfile()

 go func() {
  c := make(chan os.Signal, 1)
  signal.Notify(c, syscall.SIGINT)
  <-c

  logx.Info("profile: 捕获到中断信号, 停止分析")
  prof.Stop()

  signal.Reset()
  syscall.Kill(os.Getpid(), syscall.SIGINT)
 }()

 return &prof
}

// createDumpFile 创建一个分析文件。
func createDumpFile(kind string) string {
 command := path.Base(os.Args[0])
 pid := syscall.Getpid()
 return path.Join(os.TempDir(), fmt.Sprintf("%s-%d-%s-%s.pprof",
  command, pid, kind, time.Now().Format(timeFormat)))
}
```
