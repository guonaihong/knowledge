
### 一、客户端 拦截器

#### 1.1 客户端 拦截器例子

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

#### 1.2 实现

##### 1.2.1 拦截器链赋值

```go
func WithUnaryInterceptor(f UnaryClientInterceptor) DialOption {
 return newFuncDialOption(func(o *dialOptions) {
  o.unaryInt = f
 })
}

// WithChainUnaryInterceptor returns a DialOption that specifies the chained
// interceptor for unary RPCs. The first interceptor will be the outer most,
// while the last interceptor will be the inner most wrapper around the real call.
// All interceptors added by this method will be chained, and the interceptor
// defined by WithUnaryInterceptor will always be prepended to the chain.
func WithChainUnaryInterceptor(interceptors ...UnaryClientInterceptor) DialOption {
 return newFuncDialOption(func(o *dialOptions) {
  o.chainUnaryInts = append(o.chainUnaryInts, interceptors...)
 })
}
```

##### 1.2.2 拦截器链

```go
func chainUnaryClientInterceptors(cc *ClientConn) {
 interceptors := cc.dopts.chainUnaryInts
 // Prepend dopts.unaryInt to the chaining interceptors if it exists, since unaryInt will
 // be executed before any other chained interceptors.
 if cc.dopts.unaryInt != nil {
  interceptors = append([]UnaryClientInterceptor{cc.dopts.unaryInt}, interceptors...)
 }
 var chainedInt UnaryClientInterceptor
 if len(interceptors) == 0 {
  chainedInt = nil
 } else if len(interceptors) == 1 {
  chainedInt = interceptors[0]
 } else {
  chainedInt = func(ctx context.Context, method string, req, reply any, cc *ClientConn, invoker UnaryInvoker, opts ...CallOption) error {
   return interceptors[0](ctx, method, req, reply, cc, getChainUnaryInvoker(interceptors, 0, invoker), opts...)
  }
 }
 cc.dopts.unaryInt = chainedInt
}

func getChainUnaryInvoker(interceptors []UnaryClientInterceptor, curr int, finalInvoker UnaryInvoker) UnaryInvoker {
 if curr == len(interceptors)-1 {
  return finalInvoker
 }
 return func(ctx context.Context, method string, req, reply any, cc *ClientConn, opts ...CallOption) error {
  return interceptors[curr+1](ctx, method, req, reply, cc, getChainUnaryInvoker(interceptors, curr+1, finalInvoker), opts...)
 }
}
```

##### 1.2.3 调用端实现

```go
func (cc *ClientConn) Invoke(ctx context.Context, method string, args, reply any, opts ...CallOption) error {
 // allow interceptor to see all applicable call options, which means those
 // configured as defaults from dial option as well as per-call options
 opts = combine(cc.dopts.callOptions, opts)

 if cc.dopts.unaryInt != nil {
  return cc.dopts.unaryInt(ctx, method, args, reply, cc, invoke, opts...)
 }
 return invoke(ctx, method, args, reply, cc, opts...)
}

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
