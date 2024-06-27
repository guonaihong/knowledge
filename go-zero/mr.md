### 使用例子

```go
 err := Finish(func() error {
  atomic.AddUint32(&total, 2)
  return nil
 }, func() error {
  atomic.AddUint32(&total, 3)
  return nil
 }, func() error {
  atomic.AddUint32(&total, 5)
  return nil
 })
```

### 解析

mr的作用和errgroup很像，主要包含了两个功能

* 把业务函数放至多个goroutine中执行，所有的函数执行完并返回
* 并限并发数
* mr默认是16个goroutine中执行

### 源代码注释版本

```go
package mr

import (
 "context"
 "errors"
 "sync"
 "sync/atomic"

 "github.com/zeromicro/go-zero/core/errorx"
)

const (
 defaultWorkers = 16 // 默认工作线程数
 minWorkers     = 1  // 最小工作线程数
)

var (
 // ErrCancelWithNil 表示mapreduce被nil取消的错误
 ErrCancelWithNil = errors.New("mapreduce cancelled with nil")
 // ErrReduceNoOutput 表示reduce没有输出值的错误
 ErrReduceNoOutput = errors.New("reduce not writing value")
)

type (
 // ForEachFunc 用于处理元素，但没有输出
 ForEachFunc[T any] func(item T)
 // GenerateFunc 用于让调用者发送元素到源
 GenerateFunc[T any] func(source chan<- T)
 // MapFunc 用于处理元素并将输出写入writer
 MapFunc[T, U any] func(item T, writer Writer[U])
 // MapperFunc 用于处理元素并将输出写入writer，使用cancel函数取消处理
 MapperFunc[T, U any] func(item T, writer Writer[U], cancel func(error))
 // ReducerFunc 用于减少所有映射输出并将结果写入writer，使用cancel函数取消处理
 ReducerFunc[U, V any] func(pipe <-chan U, writer Writer[V], cancel func(error))
 // VoidReducerFunc 用于减少所有映射输出，但没有输出，使用cancel函数取消处理
 VoidReducerFunc[U any] func(pipe <-chan U, cancel func(error))
 // Option 定义了自定义mapreduce的方法
 Option func(opts *mapReduceOptions)

 mapperContext[T, U any] struct {
  ctx       context.Context
  mapper    MapFunc[T, U]
  source    <-chan T
  panicChan *onceChan
  collector chan<- U
  doneChan  <-chan struct{}
  workers   int
 }

 mapReduceOptions struct {
  ctx     context.Context
  workers int
 }

 // Writer 接口包装了Write方法
 Writer[T any] interface {
  Write(v T)
 }
)

// Finish 并行运行fns，任何一个错误都会取消
func Finish(fns ...func() error) error {
 if len(fns) == 0 {
  return nil
 }

 return MapReduceVoid(func(source chan<- func() error) {
  for _, fn := range fns {
   source <- fn
  }
 }, func(fn func() error, writer Writer[any], cancel func(error)) {
  if err := fn(); err != nil {
   cancel(err)
  }
 }, func(pipe <-chan any, cancel func(error)) {
 }, WithWorkers(len(fns)))
}

// FinishVoid 并行运行fns
func FinishVoid(fns ...func()) {
 if len(fns) == 0 {
  return
 }

 ForEach(func(source chan<- func()) {
  for _, fn := range fns {
   source <- fn
  }
 }, func(fn func()) {
  fn()
 }, WithWorkers(len(fns)))
}

// ForEach 映射所有来自给定generate的元素但没有输出
func ForEach[T any](generate GenerateFunc[T], mapper ForEachFunc[T], opts ...Option) {
 options := buildOptions(opts...)
 panicChan := &onceChan{channel: make(chan any)}
 source := buildSource(generate, panicChan)
 collector := make(chan any)
 done := make(chan struct{})

 go executeMappers(mapperContext[T, any]{
  ctx: options.ctx,
  mapper: func(item T, _ Writer[any]) {
   mapper(item)
  },
  source:    source,
  panicChan: panicChan,
  collector: collector,
  doneChan:  done,
  workers:   options.workers,
 })

 for {
  select {
  case v := <-panicChan.channel:
   panic(v)
  case _, ok := <-collector:
   if !ok {
    return
   }
  }
 }
}

// MapReduce 映射所有由给定generate函数生成的元素，并使用给定的reducer减少输出元素
func MapReduce[T, U, V any](generate GenerateFunc[T], mapper MapperFunc[T, U], reducer ReducerFunc[U, V],
 opts ...Option) (V, error) {
 panicChan := &onceChan{channel: make(chan any)}
 source := buildSource(generate, panicChan)
 return mapReduceWithPanicChan(source, panicChan, mapper, reducer, opts...)
}

// MapReduceChan 映射所有来自source的元素，并使用给定的reducer减少输出元素
func MapReduceChan[T, U, V any](source <-chan T, mapper MapperFunc[T, U], reducer ReducerFunc[U, V],
 opts ...Option) (V, error) {
 panicChan := &onceChan{channel: make(chan any)}
 return mapReduceWithPanicChan(source, panicChan, mapper, reducer, opts...)
}

// mapReduceWithPanicChan 映射所有来自source的元素，并使用给定的reducer减少输出元素
func mapReduceWithPanicChan[T, U, V any](source <-chan T, panicChan *onceChan, mapper MapperFunc[T, U],
 reducer ReducerFunc[U, V], opts ...Option) (val V, err error) {
 options := buildOptions(opts...)
 // output 用于写入最终结果
 output := make(chan V)
 defer func() {
  // reducer 只能写一次，如果多次，panic
  for range output {
   panic("more than one element written in reducer")
  }
 }()

 // collector 用于从mapper收集数据，并在reducer中消费
 collector := make(chan U, options.workers)
 // 如果done关闭，所有mapper和reducer应停止处理
 done := make(chan struct{})
 writer := newGuardedWriter(options.ctx, output, done)
 var closeOnce sync.Once
 // 使用原子类型避免数据竞争
 var retErr errorx.AtomicError
 finish := func() {
  closeOnce.Do(func() {
   close(done)
   close(output)
  })
 }
 cancel := once(func(err error) {
  if err != nil {
   retErr.Set(err)
  } else {
   retErr.Set(ErrCancelWithNil)
  }

  drain(source)
  finish()
 })

 go func() {
  defer func() {
   drain(collector)
   if r := recover(); r != nil {
    panicChan.write(r)
   }
   finish()
  }()

  reducer(collector, writer, cancel)
 }()

 go executeMappers(mapperContext[T, U]{
  ctx: options.ctx,
  mapper: func(item T, w Writer[U]) {
   mapper(item, w, cancel)
  },
  source:    source,
  panicChan: panicChan,
  collector: collector,
  doneChan:  done,
  workers:   options.workers,
 })

 select {
 case <-options.ctx.Done():
  cancel(context.DeadlineExceeded)
  err = context.DeadlineExceeded
 case v := <-panicChan.channel:
  // 在这里排空output，否则defer中的for循环会panic
  drain(output)
  panic(v)
 case v, ok := <-output:
  if e := retErr.Load(); e != nil {
   err = e
  } else if ok {
   val = v
  } else {
   err = ErrReduceNoOutput
  }
 }

 return
}

// MapReduceVoid 映射所有由给定generate生成的元素，并使用给定的reducer减少输出元素
func MapReduceVoid[T, U any](generate GenerateFunc[T], mapper MapperFunc[T, U],
 reducer VoidReducerFunc[U], opts ...Option) error {
 _, err := MapReduce(generate, mapper, func(input <-chan U, writer Writer[any], cancel func(error)) {
  reducer(input, cancel)
 }, opts...)
 if errors.Is(err, ErrReduceNoOutput) {
  return nil
 }

 return err
}

// WithContext 自定义一个接受给定ctx的mapreduce处理
func WithContext(ctx context.Context) Option {
 return func(opts *mapReduceOptions) {
  opts.ctx = ctx
 }
}

// WithWorkers 自定义一个带有给定workers的mapreduce处理
func WithWorkers(workers int) Option {
 return func(opts *mapReduceOptions) {
  if workers < minWorkers {
   opts.workers = minWorkers
  } else {
   opts.workers = workers
  }
 }
}

func buildOptions(opts ...Option) *mapReduceOptions {
 options := newOptions()
 for _, opt := range opts {
  opt(options)
 }

 return options
}

func buildSource[T any](generate GenerateFunc[T], panicChan *onceChan) chan T {
 source := make(chan T)
 go func() {
  defer func() {
   if r := recover(); r != nil {
    panicChan.write(r)
   }
   close(source)
  }()

  generate(source)
 }()

 return source
}

// drain 排空通道
func drain[T any](channel <-chan T) {
 // 排空通道
 for range channel {
 }
}

func executeMappers[T, U any](mCtx mapperContext[T, U]) {
 var wg sync.WaitGroup
 defer func() {
  wg.Wait()
  close(mCtx.collector)
  drain(mCtx.source)
 }()

 var failed int32
 pool := make(chan struct{}, mCtx.workers)
 writer := newGuardedWriter(mCtx.ctx, mCtx.collector, mCtx.doneChan)
 for atomic.LoadInt32(&failed) == 0 {
  select {
  case <-mCtx.ctx.Done():
   return
  case <-mCtx.doneChan:
   return
  case pool <- struct{}{}:
   item, ok := <-mCtx.source
   if !ok {
    <-pool
    return
   }

   wg.Add(1)
   go func() {
    defer func() {
     if r := recover(); r != nil {
      atomic.AddInt32(&failed, 1)
      mCtx.panicChan.write(r)
     }
     wg.Done()
     <-pool
    }()

    mCtx.mapper(item, writer)
   }()
  }
 }
}

func newOptions() *mapReduceOptions {
 return &mapReduceOptions{
  ctx:     context.Background(),
  workers: defaultWorkers,
 }
}

func once(fn func(error)) func(error) {
 once := new(sync.Once)
 return func(err error) {
  once.Do(func() {
   fn(err)
  })
 }
}

type guardedWriter[T any] struct {
 ctx     context.Context
 channel chan<- T
 done    <-chan struct{}
}

func newGuardedWriter[T any](ctx context.Context, channel chan<- T, done <-chan struct{}) guardedWriter[T] {
 return guardedWriter[T]{
  ctx:     ctx,
  channel: channel,
  done:    done,
 }
}

func (gw guardedWriter[T]) Write(v T) {
 select {
 case <-gw.ctx.Done():
  return
 case <-gw.done:
  return
 default:
  gw.channel <- v
 }
}

type onceChan struct {
 channel chan any
 wrote   int32
}

func (oc *onceChan) write(val any) {
 if atomic.CompareAndSwapInt32(&oc.wrote, 0, 1) {
  oc.channel <- val
 }
}
```
