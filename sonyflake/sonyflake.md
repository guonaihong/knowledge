### sonyflake

sonyflake是一个分布式唯一 ID 生成器。由三部分组成

* 时间戳
* 序列号
* 机器 ID, 由ip地址的后两位组成

需要注意的事，如果配置StartTime等于当前时间，输出的id，一般是34bit位。
如果默认的，2024年已经有59bit位，redis里面如果直接用sonyflake的id排序就会有精度问题

#### 1. 拼接id的代码

```go
 return uint64(sf.elapsedTime)<<(BitLenSequence+BitLenMachineID) |
  uint64(sf.sequence)<<BitLenMachineID |
  uint64(sf.machineID), nil
```

### 代码加上注释版本

```go
// Package sonyflake 实现了 Sonyflake，一个受 Twitter 的 Snowflake 启发的分布式唯一 ID 生成器。
//
// Sonyflake ID 由以下部分组成：
//
// 39 位用于以 10 毫秒为单位的时间
//  8 位用于序列号
// 16 位用于机器 ID
package sonyflake

import (
 "errors"
 "net"
 "sync"
 "time"

 "github.com/sony/sonyflake/types"
)

// 这些常量是 Sonyflake ID 各部分的位长度。
const (
 BitLenTime      = 39                               // 时间的位长度
 BitLenSequence  = 8                                // 序列号的位长度
 BitLenMachineID = 63 - BitLenTime - BitLenSequence // 机器 ID 的位长度
)

// Settings 配置 Sonyflake：
//
// StartTime 是从哪个时间点开始计算 Sonyflake 时间的。
// 如果 StartTime 为 0，Sonyflake 的开始时间设置为 "2014-09-01 00:00:00 +0000 UTC"。
// 如果 StartTime 在当前时间之后，Sonyflake 不会被创建。
//
// MachineID 返回 Sonyflake 实例的唯一 ID。
// 如果 MachineID 返回错误，Sonyflake 不会被创建。
// 如果 MachineID 为 nil，使用默认的 MachineID。
// 默认的 MachineID 返回私有 IP 地址的低 16 位。
//
// CheckMachineID 验证机器 ID 的唯一性。
// 如果 CheckMachineID 返回 false，Sonyflake 不会被创建。
// 如果 CheckMachineID 为 nil，不进行验证。
type Settings struct {
 StartTime      time.Time
 MachineID      func() (uint16, error)
 CheckMachineID func(uint16) bool
}

// Sonyflake 是一个分布式唯一 ID 生成器。
type Sonyflake struct {
 mutex       *sync.Mutex //锁
 startTime   int64 // 开始时间
 elapsedTime int64 // 当前时间
 sequence    uint16 // 序列号
 machineID   uint16 // 机器 ID
}

var (
 ErrStartTimeAhead   = errors.New("开始时间在当前时间之后")
 ErrNoPrivateAddress = errors.New("没有私有 IP 地址")
 ErrOverTimeLimit    = errors.New("超过时间限制")
 ErrInvalidMachineID = errors.New("无效的机器 ID")
)

var defaultInterfaceAddrs = net.InterfaceAddrs

// New 返回一个使用给定 Settings 配置的新 Sonyflake。
// New 在以下情况下返回错误：
// - Settings.StartTime 在当前时间之后。
// - Settings.MachineID 返回错误。
// - Settings.CheckMachineID 返回 false。
func New(st Settings) (*Sonyflake, error) {
 if st.StartTime.After(time.Now()) {
  return nil, ErrStartTimeAhead
 }

 sf := new(Sonyflake)
 sf.mutex = new(sync.Mutex)
 sf.sequence = uint16(1<<BitLenSequence - 1)

 if st.StartTime.IsZero() {
  sf.startTime = toSonyflakeTime(time.Date(2014, 9, 1, 0, 0, 0, 0, time.UTC))
 } else {
  sf.startTime = toSonyflakeTime(st.StartTime)
 }

 var err error
 if st.MachineID == nil {
  sf.machineID, err = lower16BitPrivateIP(defaultInterfaceAddrs)
 } else {
  sf.machineID, err = st.MachineID()
 }
 if err != nil {
  return nil, err
 }

 if st.CheckMachineID != nil && !st.CheckMachineID(sf.machineID) {
  return nil, ErrInvalidMachineID
 }

 return sf, nil
}

// NewSonyflake 返回一个使用给定 Settings 配置的新 Sonyflake。
// NewSonyflake 在以下情况下返回 nil：
// - Settings.StartTime 在当前时间之后。
// - Settings.MachineID 返回错误。
// - Settings.CheckMachineID 返回 false。
func NewSonyflake(st Settings) *Sonyflake {
 sf, _ := New(st)
 return sf
}

// NextID 生成下一个唯一的 ID。
// 当 Sonyflake 时间溢出后，NextID 返回错误。
func (sf *Sonyflake) NextID() (uint64, error) {
 const maskSequence = uint16(1<<BitLenSequence - 1)

 sf.mutex.Lock()
 defer sf.mutex.Unlock()

 current := currentElapsedTime(sf.startTime)
 if sf.elapsedTime < current {
  sf.elapsedTime = current
  sf.sequence = 0
 } else { // sf.elapsedTime >= current
  sf.sequence = (sf.sequence + 1) & maskSequence
  if sf.sequence == 0 {
   sf.elapsedTime++
   overtime := sf.elapsedTime - current
   time.Sleep(sleepTime((overtime)))
  }
 }

 return sf.toID()
}

const sonyflakeTimeUnit = 1e7 // nsec, 即 10 毫秒

func toSonyflakeTime(t time.Time) int64 {
 return t.UTC().UnixNano() / sonyflakeTimeUnit
}

func currentElapsedTime(startTime int64) int64 {
 return toSonyflakeTime(time.Now()) - startTime
}

func sleepTime(overtime int64) time.Duration {
 return time.Duration(overtime*sonyflakeTimeUnit) -
  time.Duration(time.Now().UTC().UnixNano()%sonyflakeTimeUnit)
}

func (sf *Sonyflake) toID() (uint64, error) {
 if sf.elapsedTime >= 1<<BitLenTime {
  return 0, ErrOverTimeLimit
 }

 return uint64(sf.elapsedTime)<<(BitLenSequence+BitLenMachineID) |
  uint64(sf.sequence)<<BitLenMachineID |
  uint64(sf.machineID), nil
}

func privateIPv4(interfaceAddrs types.InterfaceAddrs) (net.IP, error) {
 as, err := interfaceAddrs()
 if err != nil {
  return nil, err
 }

 for _, a := range as {
  ipnet, ok := a.(*net.IPNet)
  if !ok || ipnet.IP.IsLoopback() {
   continue
  }

  ip := ipnet.IP.To4()
  if isPrivateIPv4(ip) {
   return ip, nil
  }
 }
 return nil, ErrNoPrivateAddress
}

func isPrivateIPv4(ip net.IP) bool {
 // 允许私有 IP 地址（RFC1918）和链路本地地址（RFC3927）
 return ip != nil &&
  (ip[0] == 10 || ip[0] == 172 && (ip[1] >= 16 && ip[1] < 32) || ip[0] == 192 && ip[1] == 168 || ip[0] == 169 && ip[1] == 254)
}

func lower16BitPrivateIP(interfaceAddrs types.InterfaceAddrs) (uint16, error) {
 ip, err := privateIPv4(interfaceAddrs)
 if err != nil {
  return 0, err
 }

 return uint16(ip[2])<<8 + uint16(ip[3]), nil
}

// ElapsedTime 返回生成给定 Sonyflake ID 时的已用时间。
func ElapsedTime(id uint64) time.Duration {
 return time.Duration(elapsedTime(id) * sonyflakeTimeUnit)
}

func elapsedTime(id uint64) uint64 {
 return id >> (BitLenSequence + BitLenMachineID)
}

// SequenceNumber 返回 Sonyflake ID 的序列号。
func SequenceNumber(id uint64) uint64 {
 const maskSequence = uint64((1<<BitLenSequence - 1) << BitLenMachineID)
 return id & maskSequence >> BitLenMachineID
}

// MachineID 返回 Sonyflake ID 的机器 ID。
func MachineID(id uint64) uint64 {
 const maskMachineID = uint64(1<<BitLenMachineID - 1)
 return id & maskMachineID
}

// Decompose 返回 Sonyflake ID 的各部分。
func Decompose(id uint64) map[string]uint64 {
 msb := id >> 63
 time := elapsedTime(id)
 sequence := SequenceNumber(id)
 machineID := MachineID(id)
 return map[string]uint64{
  "id":         id,
  "msb":        msb,
  "time":       time,
  "sequence":   sequence,
  "machine-id": machineID,
 }
}
```
