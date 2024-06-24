### 使用例子

```go
package main

import (
 "fmt"
 "sync"

 "golang.org/x/sync/singleflight"
)

// 假设这是一个数据库查询函数
func getData(key string) (string, error) {
 // 模拟数据库查询
 return key + " data", nil
}

func main() {
 group := singleflight.Group{}

 // 模拟并发请求
 var wg sync.WaitGroup
 for i := 0; i < 10; i++ {
  wg.Add(1)
  go func() {
   defer wg.Done()

   // 使用 singleflight 来执行数据库查询
   val, err, shared := group.Do("key1", func() (interface{}, error) {
    return getData("key1")
   })
   if err != nil {
    fmt.Println("Error:", err)
    return
   }
   data := val.(string)
   fmt.Printf("Got data: %s, shared: %v\n", data, shared)
  }()
 }

 wg.Wait()
}
```

### 原理总结

* singleflight 包提供了一种机制，用于抑制重复的函数调用。我们常常使用redis加速查询，但是数据穿透时，访问数据库可以使用该包加速，效果是当同一个key的并发数降到1, 极大提高了性能。
* singleflight 的实现使用了sync.WaitGroup, 一次Add，可以唤醒多个Wait的特性, 第一次进入的请求去干活，别的请求，等待第一个请求结束，拿到结果

#### 实现拆分

* 第2到n次进的逻辑, 找到这个key对应的map，然后等待第一个进入的go程结束，拿到结果

```go
 if c, ok := g.m[key]; ok {
  c.dups++
  g.mu.Unlock()
  c.wg.Wait()

  if e, ok := c.err.(*panicError); ok {
   panic(e)
  } else if c.err == errGoexit {
   runtime.Goexit()
  }
  return c.val, c.err, true
 }
```

* 第1次进的逻辑

这里删除了各种错误处理的代码，第一个进入的逻辑会新建一个Call, 并把它添加到g.m中。 执行完就解锁，然后从map里面返回

```go

 c := new(call)
 c.wg.Add(1)
 g.m[key] = c
 g.mu.Unlock()

  c.wg.Done()
  if g.m[key] == c {
   delete(g.m, key)
  }
  c.val, c.err = fn()
```

### 源代码

```go
// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package singleflight 提供了一种机制，用于抑制重复的函数调用。
package singleflight // import "golang.org/x/sync/singleflight"

import (
 "bytes"
 "errors"
 "fmt"
 "runtime"
 "runtime/debug"
 "sync"
)

// errGoexit 表示在用户提供的函数中调用了 runtime.Goexit。
var errGoexit = errors.New("runtime.Goexit was called")

// panicError 是从 panic 中恢复的任意值，以及在执行给定函数期间的堆栈跟踪。
type panicError struct {
 value interface{}
 stack []byte
}

// Error 实现了 error 接口。
func (p *panicError) Error() string {
 return fmt.Sprintf("%v\n\n%s", p.value, p.stack)
}

func (p *panicError) Unwrap() error {
 err, ok := p.value.(error)
 if !ok {
  return nil
 }

 return err
}

func newPanicError(v interface{}) error {
 stack := debug.Stack()

 // 堆栈跟踪的第一行是 "goroutine N [status]:" 的形式，但在 panic 到达 Do 时，goroutine 可能不再存在，其状态可能已更改。
 // 因此，我们需要修剪掉误导性的行。
 if line := bytes.IndexByte(stack[:], '\n'); line >= 0 {
  stack = stack[line+1:]
 }
 return &panicError{value: v, stack: stack}
}

// call 是一个正在飞行或已经完成的 singleflight.Do 调用
type call struct {
 wg sync.WaitGroup

 // 这些字段在 WaitGroup 完成之前只写入一次，并且在 WaitGroup 完成后只读取。
 val interface{}
 err error

 // 这些字段在 WaitGroup 完成之前，在 singleflight 互斥锁的保护下读写，并且在 WaitGroup 完成后只读取。
 dups  int
 chans []chan<- Result
}

// Group 表示一类工作，并形成一个命名空间，在该命名空间中可以执行具有重复抑制的工作单元。
type Group struct {
 mu sync.Mutex       // 保护 m
 m  map[string]*call // 惰性初始化
}

// Result 持有 Do 的结果，以便可以通过通道传递。
type Result struct {
 Val    interface{}
 Err    error
 Shared bool
}

// Do 执行并返回给定函数的结果，确保对于给定键，只有一个执行中的调用。
// 如果有重复调用，重复的调用者将等待原始调用完成并接收相同的结果。
// 返回值 shared 指示 v 是否被多个调用者接收。
func (g *Group) Do(key string, fn func() (interface{}, error)) (v interface{}, err error, shared bool) {
 g.mu.Lock()
 if g.m == nil {
  g.m = make(map[string]*call)
 }
 if c, ok := g.m[key]; ok {
  c.dups++
  g.mu.Unlock()
  c.wg.Wait()

  if e, ok := c.err.(*panicError); ok {
   panic(e)
  } else if c.err == errGoexit {
   runtime.Goexit()
  }
  return c.val, c.err, true
 }
 c := new(call)
 c.wg.Add(1)
 g.m[key] = c
 g.mu.Unlock()

 g.doCall(c, key, fn)
 return c.val, c.err, c.dups > 0
}

// DoChan 类似于 Do，但返回一个通道，该通道将在结果准备好时接收结果。
//
// 返回的通道不会被关闭。
func (g *Group) DoChan(key string, fn func() (interface{}, error)) <-chan Result {
 ch := make(chan Result, 1)
 g.mu.Lock()
 if g.m == nil {
  g.m = make(map[string]*call)
 }
 if c, ok := g.m[key]; ok {
  c.dups++
  c.chans = append(c.chans, ch)
  g.mu.Unlock()
  return ch
 }
 c := &call{chans: []chan<- Result{ch}}
 c.wg.Add(1)
 g.m[key] = c
 g.mu.Unlock()

 go g.doCall(c, key, fn)

 return ch
}

// doCall 处理与键相关的单个调用。
func (g *Group) doCall(c *call, key string, fn func() (interface{}, error)) {
 normalReturn := false
 recovered := false

 // 使用双 defer 来区分 panic 和 runtime.Goexit，更多细节请参见 https://golang.org/cl/134395
 defer func() {
  // 给定的函数调用了 runtime.Goexit
  if !normalReturn && !recovered {
   c.err = errGoexit
  }

  g.mu.Lock()
  defer g.mu.Unlock()
  c.wg.Done()
  if g.m[key] == c {
   delete(g.m, key)
  }

  if e, ok := c.err.(*panicError); ok {
   // 为了防止等待的通道永远被阻塞，需要确保这个 panic 不能被恢复。
   if len(c.chans) > 0 {
    go panic(e)
    select {} // 保持这个 goroutine 存在，以便它将出现在崩溃转储中。
   } else {
    panic(e)
   }
  } else if c.err == errGoexit {
   // 已经在进行 goexit，无需再次调用
  } else {
   // 正常返回
   for _, ch := range c.chans {
    ch <- Result{c.val, c.err, c.dups > 0}
   }
  }
 }()

 func() {
  defer func() {
   if !normalReturn {
    // 理想情况下，我们会在确定这是 panic 还是 runtime.Goexit 之后再获取堆栈跟踪。
    //
    // 不幸的是，我们区分两者的唯一方法是查看 recover 是否阻止了 goroutine 终止，而当我们知道这一点时，与 panic 相关的堆栈跟踪部分已经被丢弃。
    if r := recover(); r != nil {
     c.err = newPanicError(r)
    }
   }
  }()

  c.val, c.err = fn()
  normalReturn = true
 }()

 if !normalReturn {
  recovered = true
 }
}

// Forget 告诉 singleflight 忘记一个键。 未来对该键的 Do 调用将调用函数，而不是等待早期的调用完成。
func (g *Group) Forget(key string) {
 g.mu.Lock()
 delete(g.m, key)
 g.mu.Unlock()
}
```
