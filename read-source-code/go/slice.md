
### 序

```go
var b []byte
len(b)

var i []int
cap(i)
```

### 一、slice 如何计算新的cap的长度

* 如果新的len大于2倍的cap，直接用len当作cap的长度
* 如果小于256，直接返回2倍的oldcap
* 1/4 *newcap + 3/4* 256。 随着newcap越大，slice就越接近1.25的系统增加容量, 想象一下y=ax+b的线条，3/4 * 256 的作用把整根线条往上提了提

```go
func nextslicecap(newLen, oldCap int) int {
 newcap := oldCap
 doublecap := newcap + newcap
 if newLen > doublecap {
  return newLen
 }

 const threshold = 256
 if oldCap < threshold {
  return doublecap
 }
 for {
  newcap += (newcap + 3*threshold) >> 2
  if uint(newcap) >= uint(newLen) {
   break
  }
 }

 if newcap <= 0 {
  return newLen
 }
 return newcap
}
```

### 带注释版本

```go
package runtime

import (
 "internal/abi"
 "internal/goarch"
 "runtime/internal/math"
 "runtime/internal/sys"
 "unsafe"
)

// slice 结构体表示一个切片，包含指向数组的指针、长度和容量
type slice struct {
 array unsafe.Pointer
 len   int
 cap   int
}

// notInHeapSlice 结构体表示一个由 runtime/internal/sys.NotInHeap 内存支持的切片
type notInHeapSlice struct {
 array *notInHeap
 len   int
 cap   int
}

// panicmakeslicelen 函数在创建切片时，如果长度超出范围，则引发 panic
func panicmakeslicelen() {
 panic(errorString("makeslice: len out of range"))
}

// panicmakeslicecap 函数在创建切片时，如果容量超出范围，则引发 panic
func panicmakeslicecap() {
 panic(errorString("makeslice: cap out of range"))
}

// makeslicecopy 函数分配一个长度为 "tolen" 的 "et" 类型元素的切片，
// 然后将 "fromlen" 个 "et" 类型元素从 "from" 复制到新分配的切片中
func makeslicecopy(et *_type, tolen int, fromlen int, from unsafe.Pointer) unsafe.Pointer {
 var tomem, copymem uintptr
 if uintptr(tolen) > uintptr(fromlen) {
  var overflow bool
  tomem, overflow = math.MulUintptr(et.Size_, uintptr(tolen))
  if overflow || tomem > maxAlloc || tolen < 0 {
   panicmakeslicelen()
  }
  copymem = et.Size_ * uintptr(fromlen)
 } else {
  // fromlen 是一个已知的好长度，提供且等于或大于 tolen，
  // 因此 tolen 也是一个好的切片长度，因为 from 和 to 切片具有相同的元素宽度
  tomem = et.Size_ * uintptr(tolen)
  copymem = tomem
 }

 var to unsafe.Pointer
 if et.PtrBytes == 0 {
  to = mallocgc(tomem, nil, false)
  if copymem < tomem {
   memclrNoHeapPointers(add(to, copymem), tomem-copymem)
  }
 } else {
  // 注意：不能使用 rawmem（它避免了内存的零初始化），因为 GC 可以扫描未初始化的内存
  to = mallocgc(tomem, et, true)
  if copymem > 0 && writeBarrier.enabled {
   // 仅对 old.array 中的指针进行着色，因为我们知道目标切片 to
   // 仅包含 nil 指针，因为它在分配期间已被清除
   bulkBarrierPreWriteSrcOnly(uintptr(to), uintptr(from), copymem, et)
  }
 }

 if raceenabled {
  callerpc := getcallerpc()
  pc := abi.FuncPCABIInternal(makeslicecopy)
  racereadrangepc(from, copymem, callerpc, pc)
 }
 if msanenabled {
  msanread(from, copymem)
 }
 if asanenabled {
  asanread(from, copymem)
 }

 memmove(to, from, copymem)

 return to
}

// makeslice 函数分配一个长度为 "len" 和容量为 "cap" 的 "et" 类型元素的切片
func makeslice(et *_type, len, cap int) unsafe.Pointer {
 mem, overflow := math.MulUintptr(et.Size_, uintptr(cap))
 if overflow || mem > maxAlloc || len < 0 || len > cap {
  // 注意：当有人执行 make([]T, bignumber) 时，产生一个 'len out of range' 错误，而不是 'cap out of range' 错误
  mem, overflow := math.MulUintptr(et.Size_, uintptr(len))
  if overflow || mem > maxAlloc || len < 0 {
   panicmakeslicelen()
  }
  panicmakeslicecap()
 }

 return mallocgc(mem, et, true)
}

// makeslice64 函数分配一个长度为 "len64" 和容量为 "cap64" 的 "et" 类型元素的切片
func makeslice64(et *_type, len64, cap64 int64) unsafe.Pointer {
 len := int(len64)
 if int64(len) != len64 {
  panicmakeslicelen()
 }

 cap := int(cap64)
 if int64(cap) != cap64 {
  panicmakeslicecap()
 }

 return makeslice(et, len, cap)
}

// growslice 函数为切片分配新的后备存储
func growslice(oldPtr unsafe.Pointer, newLen, oldCap, num int, et *_type) slice {
 oldLen := newLen - num
 if raceenabled {
  callerpc := getcallerpc()
  racereadrangepc(oldPtr, uintptr(oldLen*int(et.Size_)), callerpc, abi.FuncPCABIInternal(growslice))
 }
 if msanenabled {
  msanread(oldPtr, uintptr(oldLen*int(et.Size_)))
 }
 if asanenabled {
  asanread(oldPtr, uintptr(oldLen*int(et.Size_)))
 }

 if newLen < 0 {
  panic(errorString("growslice: len out of range"))
 }

 if et.Size_ == 0 {
  // append 不应该创建一个具有 nil 指针但非零长度的切片
  return slice{unsafe.Pointer(&zerobase), newLen, newLen}
 }

 newcap := nextslicecap(newLen, oldCap)

 var overflow bool
 var lenmem, newlenmem, capmem uintptr
 // 针对 et.Size 的常见值进行优化
 switch {
 case et.Size_ == 1:
  lenmem = uintptr(oldLen)
  newlenmem = uintptr(newLen)
  capmem = roundupsize(uintptr(newcap), noscan)
  overflow = uintptr(newcap) > maxAlloc
  newcap = int(capmem)
 case et.Size_ == goarch.PtrSize:
  lenmem = uintptr(oldLen) * goarch.PtrSize
  newlenmem = uintptr(newLen) * goarch.PtrSize
  capmem = roundupsize(uintptr(newcap)*goarch.PtrSize, noscan)
  overflow = uintptr(newcap) > maxAlloc/goarch.PtrSize
  newcap = int(capmem / goarch.PtrSize)
 case isPowerOfTwo(et.Size_):
  var shift uintptr
  if goarch.PtrSize == 8 {
   shift = uintptr(sys.TrailingZeros64(uint64(et.Size_))) & 63
  } else {
   shift = uintptr(sys.TrailingZeros32(uint32(et.Size_))) & 31
  }
  lenmem = uintptr(oldLen) << shift
  newlenmem = uintptr(newLen) << shift
  capmem = roundupsize(uintptr(newcap)<<shift, noscan)
  overflow = uintptr(newcap) > (maxAlloc >> shift)
  newcap = int(capmem >> shift)
  capmem = uintptr(newcap) << shift
 default:
  lenmem = uintptr(oldLen) * et.Size_
  newlenmem = uintptr(newLen) * et.Size_
  capmem, overflow = math.MulUintptr(et.Size_, uintptr(newcap))
  capmem = roundupsize(capmem, noscan)
  newcap = int(capmem / et.Size_)
  capmem = uintptr(newcap) * et.Size_
 }

 if overflow || capmem > maxAlloc {
  panic(errorString("growslice: len out of range"))
 }

 var p unsafe.Pointer
 if et.PtrBytes == 0 {
  p = mallocgc(capmem, nil, false)
  memclrNoHeapPointers(add(p, newlenmem), capmem-newlenmem)
 } else {
  p = mallocgc(capmem, et, true)
  if lenmem > 0 && writeBarrier.enabled {
   bulkBarrierPreWriteSrcOnly(uintptr(p), uintptr(oldPtr), lenmem-et.Size_+et.PtrBytes, et)
  }
 }
 memmove(p, oldPtr, lenmem)

 return slice{p, newLen, newcap}
}

// nextslicecap 函数计算下一个合适的切片长度
func nextslicecap(newLen, oldCap int) int {
 newcap := oldCap
 doublecap := newcap + newcap
 if newLen > doublecap {
  return newLen
 }

 const threshold = 256
 if oldCap < threshold {
    // newlen = 2 * oldcap
  return doublecap
 }
 for {
    // 随着newcap越来越大，相等于 newcap = 1.25 * newcap
  newcap += (newcap + 3*threshold) >> 2
  if uint(newcap) >= uint(newLen) {
   break
  }
 }

 if newcap <= 0 {
  return newLen
 }
 return newcap
}

// reflect_growslice 函数由 reflect 包调用，用于扩展切片
func reflect_growslice(et *_type, old slice, num int) slice {
 num -= old.cap - old.len
 new := growslice(old.array, old.cap+num, old.cap, num, et)
 if et.PtrBytes == 0 {
  oldcapmem := uintptr(old.cap) * et.Size_
  newlenmem := uintptr(new.len) * et.Size_
  memclrNoHeapPointers(add(new.array, oldcapmem), newlenmem-oldcapmem)
 }
 new.len = old.len
 return new
}

// isPowerOfTwo 函数检查一个数是否是 2 的幂
func isPowerOfTwo(x uintptr) bool {
 return x&(x-1) == 0
}

// slicecopy 函数用于将一个字符串或无指针元素的切片复制到另一个切片
func slicecopy(toPtr unsafe.Pointer, toLen int, fromPtr unsafe.Pointer, fromLen int, width uintptr) int {
 if fromLen == 0 || toLen == 0 {
  return 0
 }

 n := fromLen
 if toLen < n {
  n = toLen
 }

 if width == 0 {
  return n
 }

 size := uintptr(n) * width
 if raceenabled {
  callerpc := getcallerpc()
  pc := abi.FuncPCABIInternal(slicecopy)
  racereadrangepc(fromPtr, size, callerpc, pc)
  racewriterangepc(toPtr, size, callerpc, pc)
 }
 if msanenabled {
  msanread(fromPtr, size)
  msanwrite(toPtr, size)
 }
 if asanenabled {
  asanread(fromPtr, size)
  asanwrite(toPtr, size)
 }

 if size == 1 {
  *(*byte)(toPtr) = *(*byte)(fromPtr)
 } else {
  memmove(toPtr, fromPtr, size)
 }
 return n
}

// bytealg_MakeNoZero 函数由 internal/bytealg 包调用，用于创建一个非零初始化的字节切片
func bytealg_MakeNoZero(len int) []byte {
 if uintptr(len) > maxAlloc {
  panicmakeslicelen()
 }
 return unsafe.Slice((*byte)(mallocgc(uintptr(len), nil, false)), len)
}
```

```go
// roundupsize 函数将给定的内存大小向上舍入到合适的大小，以满足 Go 运行时内存分配器的要求。
func roundupsize(size uintptr, noscan bool) (reqSize uintptr) {
    reqSize = size // 初始化 reqSize 为输入的 size

    // 如果请求的内存大小是小对象（small object）
    if reqSize <= maxSmallSize-mallocHeaderSize {
        // 如果对象不是 noscan（即需要扫描的），并且大小超过了分配最小头信息的阈值
        if !noscan && reqSize > minSizeForMallocHeader { // !noscan && !heapBitsInSpan(reqSize)
            // 增加额外的 mallocHeaderSize 以存储额外的头部信息
            reqSize += mallocHeaderSize
        }
        // 计算需要舍入的额外字节数，如果已经包含了 mallocHeaderSize，则需要减去
        // 因为我们将在 mallocgc 中再次添加它
        if reqSize <= smallSizeMax-8 {
            // 对于小对象，使用 smallSizeDiv 来计算需要上舍入到的 class 并获取对应的 size
            return uintptr(class_to_size[size_to_class8[divRoundUp(reqSize, smallSizeDiv)]]) - (reqSize - size)
        }
        // 对于较大的对象，但仍然小于 maxSmallSize 的对象
        return uintptr(class_to_size[size_to_class128[divRoundUp(reqSize-smallSizeMax, largeSizeDiv)]]) - (reqSize - size)
    }

    // 对于大对象（large object），需要将 reqSize 对齐到下一个页面的边界
    // 首先在 reqSize 上加上 pageSize - 1，以确保对齐
    reqSize += pageSize - 1
    // 检查是否发生溢出
    if reqSize < size {
        // 如果溢出，返回原始 size
        return size
    }
    // 使用 &^ 操作来确保 reqSize 被正确地对齐到页面大小
    return reqSize &^ (pageSize - 1)
}
```
