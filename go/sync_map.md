* sync.Map现有实现
sync.Map由两个map组成，只读map和写入map, 从形式上也类似于redis前面套个map，也有缓存穿透的处理
核心数据结构

```go
type Map struct {
 // 锁
 mu Mutex

 // 类似redis, 数据的子集
 read atomic.Pointer[readOnly]

 // 类似于mysql, 数据的超集
 dirty map[any]*entry

 // 数据穿透的次数
 misses int
}
```

* sync.Map 加载后端map到read map里面的逻辑

```go
func (m *Map) missLocked() {
 m.misses++
 if m.misses < len(m.dirty) {
  return
 }
 m.read.Store(&readOnly{m: m.dirty})
 m.dirty = nil
 m.misses = 0
}
```

比较难理解的一个核心点是，在`Store`函数里面，有时候只修改了read map，这是为啥？

具体代码位于 `if v, ok := e.trySwap(&value); ok {` 只里只修改了read map的value。

因为 read和dirty的数据的value是共享的，持有同一份指针，只要修改了read，dirty的数据也会被修改

从dirtyLocked这个函数可以看到原因

```go
func (m *Map) dirtyLocked() {
 if m.dirty != nil {
  return
 }

 read := m.loadReadOnly()
 m.dirty = make(map[any]*entry, len(read.m))
 for k, e := range read.m {
  if !e.tryExpungeLocked() {
   m.dirty[k] = e
  }
 }
}
```

```go
func (m *Map) Swap(key, value any) (previous any, loaded bool) {
 read := m.loadReadOnly()
 if e, ok := read.m[key]; ok {
  if v, ok := e.trySwap(&value); ok {
   if v == nil {
    return nil, false
   }
   return *v, true
  }
 }

 m.mu.Lock()
 read = m.loadReadOnly()
 if e, ok := read.m[key]; ok {
  if e.unexpungeLocked() {
   // The entry was previously expunged, which implies that there is a
   // non-nil dirty map and this entry is not in it.
   m.dirty[key] = e
  }
  if v := e.swapLocked(&value); v != nil {
   loaded = true
   previous = *v
  }
 } else if e, ok := m.dirty[key]; ok {
  if v := e.swapLocked(&value); v != nil {
   loaded = true
   previous = *v
  }
 } else {
  if !read.amended {
   // We're adding the first new key to the dirty map.
   // Make sure it is allocated and mark the read-only map as incomplete.
   m.dirtyLocked()
   m.read.Store(&readOnly{m: read.m, amended: true})
  }
  m.dirty[key] = newEntry(value)
 }
 m.mu.Unlock()
 return previous, loaded
}
```
