### 客户端流拦截器

```go
func streamClientInterceptor(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
    log.Printf("Before streaming RPC: %s", method)
    
    clientStream, err := streamer(ctx, desc, cc, method, opts...)
    
    return &wrappedClientStream{clientStream}, err
}

type wrappedClientStream struct {
    grpc.ClientStream
}

func (w *wrappedClientStream) RecvMsg(m interface{}) error {
    log.Printf("Receive a message")
    return w.ClientStream.RecvMsg(m)
}

func (w *wrappedClientStream) SendMsg(m interface{}) error {
    log.Printf("Send a message")
    return w.ClientStream.SendMsg(m)
}

// 使用拦截器创建客户端连接
conn, err := grpc.Dial(address, grpc.WithStreamInterceptor(streamClientInterceptor))
```

### 服务端流拦截器

```go
func streamServerInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
    log.Printf("Before streaming RPC: %s", info.FullMethod)
    
    err := handler(srv, &wrappedServerStream{ss})
    
    log.Printf("After streaming RPC: %s", info.FullMethod)
    return err
}

type wrappedServerStream struct {
    grpc.ServerStream
}

func (w *wrappedServerStream) RecvMsg(m interface{}) error {
    log.Printf("Receive a message")
    return w.ServerStream.RecvMsg(m)
}

func (w *wrappedServerStream) SendMsg(m interface{}) error {
    log.Printf("Send a message")
    return w.ServerStream.SendMsg(m)
}

// 使用拦截器创建 gRPC 服务器
server := grpc.NewServer(grpc.StreamInterceptor(streamServerInterceptor))
```
