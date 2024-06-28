
### 客户端 拦截器

```go
func unaryClientInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
    // 前置逻辑
    log.Printf("Before RPC: %s", method)
    
    // 调用 RPC
    err := invoker(ctx, method, req, reply, cc, opts...)
    
    // 后置逻辑
    log.Printf("After RPC: %s", method)
    
    return err
}

// 使用拦截器创建客户端连接
conn, err := grpc.Dial(address, grpc.WithUnaryInterceptor(unaryClientInterceptor))
```

### 服务端 拦截器

```go
func unaryServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    // 前置逻辑
    log.Printf("Before RPC: %s", info.FullMethod)
    
    // 处理请求
    resp, err := handler(ctx, req)
    
    // 后置逻辑
    log.Printf("After RPC: %s", info.FullMethod)
    
    return resp, err
}

// 使用拦截器创建 gRPC 服务器
server := grpc.NewServer(grpc.UnaryInterceptor(unaryServerInterceptor))
```
