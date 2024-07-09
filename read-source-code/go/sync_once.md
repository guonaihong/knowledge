## 引

sync.Once包里面的方法是用来保证某个方法只执行一次的方法。
现在包含三个函数

* Do
* OnceFunc
* OnceValue

### 一、Do

#### 1.1 Do用法

```go
func main() {
    var once sync.Once
    var wg sync.WaitGroup

    wg.Add(10)
    defer wg.Wait()

    for i := 0; i < 10; i++ {
        go func() {
            defer wg.Done()
            once.Do(func() {
                fmt.Println("do once")
            })
        }()
    }
}
```

#### 1.2 源代码

1. 先通过原子变量确定状态，如果设置过了，直接返回
1. 接下来的情况直接加锁，防止有多个goroutine同时执行，再判断状态，再执行函数

```go
type Once struct {
 // done 表示动作是否已经执行。
 // 它位于结构体的第一位，因为在热路径中经常使用。
 // 热路径在每个调用点都被内联。
 // 将 done 放在第一位可以在某些架构（如 amd64/386）上允许更紧凑的指令，
 // 在其他架构上减少计算偏移的指令数量。
 done atomic.Uint32
 m    Mutex
}

// Do 方法确保函数 f 仅在 Once 实例的首次调用时执行。
// 换句话说，如果多次调用 once.Do(f)，只有第一次调用会执行 f，
// 即使每次调用 f 的值不同。每个要执行的函数需要一个新的 Once 实例。
// Do 方法用于必须恰好执行一次的初始化。
// 由于 f 是无参数的，可能需要使用函数字面量来捕获要通过 Do 调用的函数的参数：
// config.once.Do(func() { config.init(filename) })
// 由于 Do 调用直到 f 返回后才返回，如果 f 导致 Do 再次被调用，将会发生死锁。
// 如果 f 发生 panic，Do 认为它已经返回；未来的 Do 调用将不再调用 f。
func (o *Once) Do(f func()) {
 // 注意：以下是 Do 的一个错误实现：
 //
 // if o.done.CompareAndSwap(0, 1) {
 //  f()
 // }
 //
 // Do 保证当它返回时，f 已经完成。
 // 这个实现不会实现这个保证：
 // 如果有两个同时调用，CAS 的赢家会调用 f，而第二个会立即返回，
 // 不会等待第一个对 f 的调用完成。
 // 这就是为什么慢路径会回退到互斥锁，以及为什么 o.done.Store 必须在 f 返回后延迟执行。

 if o.done.Load() == 0 {
  // 定义慢路径以允许内联快路径。
  o.doSlow(f)
 }
}

func (o *Once) doSlow(f func()) {
 o.m.Lock()
 defer o.m.Unlock()
 if o.done.Load() == 0 {
  // 延迟存储 done 直到 f 执行完毕。
  defer o.done.Store(1)
  f()
 }
}
```

### 二、OnceFunc

#### 2.1 OnceFunc用法

```go
func main() {
    var wg sync.WaitGroup
    f := sync.OnceFunc(func() {
        fmt.Println("do once")
    })

    wg.Add(10)
    defer wg.Wait()
    for i := 0; i < 10; i++ {
        go func() {
            defer wg.Done()
            f()
        }()
    }
}
```

#### 2.2 OnceFunc第二个panic的作用

```go
func main() {
 var wg sync.WaitGroup
 f := sync.OnceFunc(func() {
  fmt.Println("do once")
  panic("wwow")
 })

 wg.Add(10)
 defer wg.Wait()
 for i := 0; i < 10; i++ {
  go func() {
   defer wg.Done()
   defer func() {
    if err := recover(); err != nil {
     fmt.Printf("%#v\n", err)
     // fmt.Printf("%s\n", debug.Stack())
    }
   }()
   f()
  }()
 }
}
```

#### 2.2 源代码

OnceFunc 用于只执行一次的函数，返回一个闭包函数

```go
func OnceFunc(f func()) func() {
 var (
  once  Once
  valid bool
  p     any
 )
 // Construct the inner closure just once to reduce costs on the fast path.
 g := func() {
  defer func() {
   p = recover()
   if !valid {
    // Re-panic immediately so on the first call the user gets a
    // complete stack trace into f.
    panic(p)
   }
  }()
  f()
  f = nil      // Do not keep f alive after invoking it.
  valid = true // Set only if f does not panic.
 }

 // 返回一个闭包函数，只执行一次
 return func() {
  once.Do(g)
  if !valid { // 如果外层函数recover了，这里可以持久抛出panic
   panic(p)
  }
 }
}
```

### 三、OnceValue

#### 3.1 OnceValue用法

```go
func main() {
 var wg sync.WaitGroup
 var count int32
 f := sync.OnceValue(func() string {
  atomic.AddInt32(&count, 1)
  return fmt.Sprint("count: ", atomic.LoadInt32(&count))
 })

 wg.Add(10)
 defer wg.Wait()
 for i := 0; i < 10; i++ {
  go func() {
   defer wg.Done()

   fmt.Println(f())
  }()
 }
}
```

#### 3.1 OnceValue实现

OnceValue 和OnceFunc类似，只是带有返回值，这里可以看下go里面的泛型是如何声明的

```go
func OnceValue[T any](f func() T) func() T {
 var (
  once   Once
  valid  bool
  p      any
  result T
 )
 g := func() {
  defer func() {
   p = recover()
   if !valid {
    panic(p)
   }
  }()
  result = f()
  f = nil
  valid = true
 }
 return func() T {
  once.Do(g)
  if !valid {
   panic(p)
  }
  return result
 }
}
```
