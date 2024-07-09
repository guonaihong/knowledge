### go-zero并发控制

默认的并发控制，是基于chan实现的，来的请求向chan写数据，申请资格，离开的时候从chan读数据

* 配置文件

```yaml
MaxConns: 100
```

* 加载的中间件

```go
func MaxConnsHandler(n int) func(http.Handler) http.Handler {
 if n <= 0 {
  return func(next http.Handler) http.Handler {
   return next
  }
 }

 return func(next http.Handler) http.Handler {
  latch := syncx.NewLimit(n)

  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
   if latch.TryBorrow() {
    defer func() {
     if err := latch.Return(); err != nil {
      logx.WithContext(r.Context()).Error(err)
     }
    }()

    next.ServeHTTP(w, r)
   } else {
    internal.Errorf(r, "concurrent connections over %d, rejected with code %d",
     n, http.StatusServiceUnavailable)
    w.WriteHeader(http.StatusServiceUnavailable)
   }
  })
 }
}
```
