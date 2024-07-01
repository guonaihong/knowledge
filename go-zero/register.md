### 服务注册流程

#### 一、etcd

#### 1.1 获取服务的IP

注册流程分为两步，第一步获取本机的IP，先获取POD_IP环境变量的值，如果没有，则从网卡列表获取第一个IP的值

```go

func figureOutListenOn(listenOn string) string {
 fields := strings.Split(listenOn, ":")
 if len(fields) == 0 {
  return listenOn
 }

 host := fields[0]
 if len(host) > 0 && host != allEths {
  return listenOn
 }

// 获取POD_IP环境变量的值, k8s会自动注册
 ip := os.Getenv(envPodIp)
 if len(ip) == 0 {
    // 获取网卡列表的值
  ip = netx.InternalIp()
 }
 if len(ip) == 0 {
  return listenOn
 }

 return strings.Join(append([]string{ip}, fields[1:]...), ":")
}

```

```go
func InternalIp() string {
 infs, err := net.Interfaces()
 if err != nil {
  return ""
 }

 for _, inf := range infs {
  if isEthDown(inf.Flags) || isLoopback(inf.Flags) {
   continue
  }

  addrs, err := inf.Addrs()
  if err != nil {
   continue
  }

  for _, addr := range addrs {
   if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
    if ipnet.IP.To4() != nil {
     return ipnet.IP.String()
    }
   }
  }
 }

 return ""
}

func isEthDown(f net.Flags) bool {
 return f&net.FlagUp != net.FlagUp
}

func isLoopback(f net.Flags) bool {
 return f&net.FlagLoopback == net.FlagLoopback
}
```

##### 1.2 服务注册

```yaml
Name: add.rpc
ListenOn: 0.0.0.0:8080
Etcd:
  Hosts:
  - 192.168.1.7:2379
  Key: add.rpc
```

* 服务注册的key是由 Etcd.Key + "/" + 租约ID组成, 比如这样add.rpc/1366719810916819729
* value是本机服务的ip地址组成, 比如这样, 192.168.31.147:8080

```go
func (p *Publisher) register(client internal.EtcdClient) (clientv3.LeaseID, error) {
    // 获取租约 lease
 resp, err := client.Grant(client.Ctx(), TimeToLive)
 if err != nil {
  return clientv3.NoLease, err
 }

 lease := resp.ID
 // 拼接key
 if p.id > 0 {
  p.fullKey = makeEtcdKey(p.key, p.id)
 } else {
  p.fullKey = makeEtcdKey(p.key, int64(lease))
 }
 // 3. 写入值到etcd里面
 _, err = client.Put(client.Ctx(), p.fullKey, p.value, clientv3.WithLease(lease))

 return lease, err
}

```

#### k8s
