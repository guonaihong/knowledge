* 用法

```go
var wg sync.WaitGroup
defer wg.Wait
wg.Add(1)
go func() {
  defer wg.Done()
  // do something
}()

```

* 原理

sync.WaitGroup 主要是基本计数和信号量实现的。  
计数为0时，唤醒wg.Wait()函数. 注意下这里面低4字节是存放等待的个数(这个问题在后面讲下)

```go
type WaitGroup struct {
 noCopy noCopy

 state atomic.Uint64 // high 32 bits are counter, low 32 bits are waiter count.
 sema  uint32
}
```

runtime_Semacquire 执行 P 操作：  
S = S - 1
判断  
    S < 0 阻塞
    S = 0 直接返回

当一个 Goroutine 调用 runtime_Semacquire 时，它会检查信号量的计数值。如果计数值大于 0 (count > 0)，它将减少该值并继续执行。如果计数值等于 0 (count == 0) 或更小，Goroutine 将被阻塞，直到其他 Goroutine 执行 runtime_Semrelease 操作，增加信号量的计数值，从而允许它继续执行。

runtime_Semrelease 执行 V 操作：  
S = S + 1
判断
如果 S > 0，则直接返回  
如果 S <=0， 释放阻塞队列中的第一个等待进程

当一个 Goroutine 调用 runtime_Semrelease 时，它会增加信号量的计数值。如果有其他 Goroutine 因为信号量的计数值等于 0 (count == 0) 或更小而被阻塞，增加计数值可能会唤醒其中一个等待的 Goroutine，使其能够继续执行 runtime_Semacquire 之后的代码。

* 聊下低4个字节的作用  
先看下这个例子，相比上面的例子，这里调用了两个`wg.Wait()`, 希望是所有的go程结果，在两个地方可以知道。
所以这里低4个字节是存放等待的个数`wg.Wait()`的个数

```go
func main() {
 var wg sync.WaitGroup
 const concurrency = 10000 // 假设我们有1万个goroutines

 // 启动100万个goroutines，每个都执行一个简单的任务
 for i := 0; i < concurrency; i++ {
  wg.Add(1) // 增加WaitGroup的计数器
  go func() {
   // 模拟一些工作
   doSomeWork()
   wg.Done() // 完成任务后减少计数器
  }()
 }

 // 等待所有goroutines完成
 go func() {
  wg.Wait()
  fmt.Println("1.所有goroutines都已完成。")
 }()
 wg.Wait()
 fmt.Println("2.所有goroutines都已完成。")
}

func doSomeWork() {
 time.Sleep(time.Second * 3)
 // 这里可以放置goroutines需要执行的工作
}
```
