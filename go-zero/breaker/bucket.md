```go
package breaker

const (
 success = iota // 成功状态
 fail           // 失败状态
 drop           // 丢弃状态
)

// bucket 定义了一个存储总和和添加次数的桶
type bucket struct {
 Sum     int64 // 总和
 Success int64 // 成功次数
 Failure int64 // 失败次数
 Drop    int64 // 丢弃次数
}

// Add 向桶中添加一个值
func (b *bucket) Add(v int64) {
 switch v {
 case fail:
  b.fail() // 添加失败
 case drop:
  b.drop() // 添加丢弃
 default:
  b.succeed() // 添加成功
 }
}

// Reset 重置桶的状态
func (b *bucket) Reset() {
 b.Sum = 0
 b.Success = 0
 b.Failure = 0
 b.Drop = 0
}

// drop 增加丢弃次数
func (b *bucket) drop() {
 b.Sum++
 b.Drop++
}

// fail 增加失败次数
func (b *bucket) fail() {
 b.Sum++
 b.Failure++
}

// succeed 增加成功次数
func (b *bucket) succeed() {
 b.Sum++
 b.Success++
}
```
