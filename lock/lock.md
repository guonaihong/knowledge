
### redis分布式锁实现

实现分布式锁，主要是实现lock和unlock

* lock
如果不存在则设置值，存在则不设置值。

```bash
SET mykey "随机值" NX PX 5000
```

* unlock
使用redis的del命令删除锁, 需要使用lua脚本实现

```go
var deleteScript = redis.NewScript(1, `
 local val = redis.call("GET", KEYS[1])
 if val == ARGV[1] then
  return redis.call("DEL", KEYS[1])
 elseif val == false then
  return -1
 else
  return 0
 end
`)
```

为什么value随机值?
是为了解决误删除的问题，假调A获得锁，这时候redis重启，数据丢了。B也获得锁，这时候A unlock，直接删除了这个key，B的访问就不是并发安全的。

下面是golang分布锁开源项目redsync，生成value值的代码，也可以用uuid实现，只是下面的方式省些内存

```go
func genValue() (string, error) {
 b := make([]byte, 16)
 _, err := rand.Read(b)
 if err != nil {
  return "", err
 }
 return base64.StdEncoding.EncodeToString(b), nil
}
```

* 续期
如果一个业务正常处理时间是1秒，少部分情况是3s，极端情况是30s，想实现Lock的时候加2s的超时时间，2s的时间到了，再加10s的超时时间，该如何实现？
redsync是通过以下两种方式实现，两个方式的唯一区别，是第一种方式容错性更高，比如redis集群重启，也可以正常处理。

```go
var touchWithSetNXScript = redis.NewScript(1, `
 if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
 elseif redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2], "NX") then
  return 1
 else
  return 0
 end
`)

var touchScript = redis.NewScript(1, `
 if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
 else
  return 0
 end
`)
```
