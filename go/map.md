### golang map

#### 负载因子

负载因子是13， 元素为bucket长度的80%开始扩容

#### map的bucket结构

golang 的map的bucket结构如下
8个top
8个key
8个value
overflow 指针

#### map访问

在访问这个函数可以清楚地看到。b.overflow是一个指针，一个bucket是8个元素。如果都没有找到，再看下overflow指针有没有值，有值的话继续往下找

```go
func mapaccessK(t *maptype, h *hmap, key unsafe.Pointer) (unsafe.Pointer, unsafe.Pointer) {
 if h == nil || h.count == 0 {
  return nil, nil
 }
 hash := t.Hasher(key, uintptr(h.hash0))
 m := bucketMask(h.B)
 b := (*bmap)(add(h.buckets, (hash&m)*uintptr(t.BucketSize)))
 if c := h.oldbuckets; c != nil {
  if !h.sameSizeGrow() {
   // There used to be half as many buckets; mask down one more power of two.
   m >>= 1
  }
  oldb := (*bmap)(add(c, (hash&m)*uintptr(t.BucketSize)))
  if !evacuated(oldb) {
   b = oldb
  }
 }
 top := tophash(hash)
bucketloop:
 for ; b != nil; b = b.overflow(t) {
  for i := uintptr(0); i < bucketCnt; i++ {
   if b.tophash[i] != top {
    if b.tophash[i] == emptyRest {
     break bucketloop
    }
    continue
   }
   k := add(unsafe.Pointer(b), dataOffset+i*uintptr(t.KeySize))
   if t.IndirectKey() {
    k = *((*unsafe.Pointer)(k))
   }
   if t.Key.Equal(key, k) {
    e := add(unsafe.Pointer(b), dataOffset+bucketCnt*uintptr(t.KeySize)+i*uintptr(t.ValueSize))
    if t.IndirectElem() {
     e = *((*unsafe.Pointer)(e))
    }
    return k, e
   }
  }
 }
 return nil, nil
}
```

#### map的扩容机制

* hashGrow: 分配新bucket的长度
* growWork: 每次access 或者delete的时候，需把老的bucket的数据复制到新的bucket
每次迁移一个桶的元素, 使用均摊的思路，这样保证每次的耗时都比较低，这点和redis的hashmap实现类似

```go
func evacuate(t *maptype, h *hmap, oldbucket uintptr) {
 b := (*bmap)(add(h.oldbuckets, oldbucket*uintptr(t.BucketSize)))
 newbit := h.noldbuckets()
 if !evacuated(b) {
  // TODO: reuse overflow buckets instead of using new ones, if there
  // is no iterator using the old buckets.  (If !oldIterator.)

  // xy contains the x and y (low and high) evacuation destinations.
  var xy [2]evacDst
  x := &xy[0]
  x.b = (*bmap)(add(h.buckets, oldbucket*uintptr(t.BucketSize)))
  x.k = add(unsafe.Pointer(x.b), dataOffset)
  x.e = add(x.k, bucketCnt*uintptr(t.KeySize))

  if !h.sameSizeGrow() {
   // Only calculate y pointers if we're growing bigger.
   // Otherwise GC can see bad pointers.
   y := &xy[1]
   y.b = (*bmap)(add(h.buckets, (oldbucket+newbit)*uintptr(t.BucketSize)))
   y.k = add(unsafe.Pointer(y.b), dataOffset)
   y.e = add(y.k, bucketCnt*uintptr(t.KeySize))
  }

  for ; b != nil; b = b.overflow(t) {
   k := add(unsafe.Pointer(b), dataOffset)
   e := add(k, bucketCnt*uintptr(t.KeySize))
   for i := 0; i < bucketCnt; i, k, e = i+1, add(k, uintptr(t.KeySize)), add(e, uintptr(t.ValueSize)) {
    top := b.tophash[i]
    if isEmpty(top) {
     b.tophash[i] = evacuatedEmpty
     continue
    }
    if top < minTopHash {
     throw("bad map state")
    }
    k2 := k
    if t.IndirectKey() {
     k2 = *((*unsafe.Pointer)(k2))
    }
    var useY uint8
    if !h.sameSizeGrow() {
     // Compute hash to make our evacuation decision (whether we need
     // to send this key/elem to bucket x or bucket y).
     hash := t.Hasher(k2, uintptr(h.hash0))
     if h.flags&iterator != 0 && !t.ReflexiveKey() && !t.Key.Equal(k2, k2) {
      // If key != key (NaNs), then the hash could be (and probably
      // will be) entirely different from the old hash. Moreover,
      // it isn't reproducible. Reproducibility is required in the
      // presence of iterators, as our evacuation decision must
      // match whatever decision the iterator made.
      // Fortunately, we have the freedom to send these keys either
      // way. Also, tophash is meaningless for these kinds of keys.
      // We let the low bit of tophash drive the evacuation decision.
      // We recompute a new random tophash for the next level so
      // these keys will get evenly distributed across all buckets
      // after multiple grows.
      useY = top & 1
      top = tophash(hash)
     } else {
      if hash&newbit != 0 {
       useY = 1
      }
     }
    }

    if evacuatedX+1 != evacuatedY || evacuatedX^1 != evacuatedY {
     throw("bad evacuatedN")
    }

    b.tophash[i] = evacuatedX + useY // evacuatedX + 1 == evacuatedY
    dst := &xy[useY]                 // evacuation destination

    if dst.i == bucketCnt {
     dst.b = h.newoverflow(t, dst.b)
     dst.i = 0
     dst.k = add(unsafe.Pointer(dst.b), dataOffset)
     dst.e = add(dst.k, bucketCnt*uintptr(t.KeySize))
    }
    dst.b.tophash[dst.i&(bucketCnt-1)] = top // mask dst.i as an optimization, to avoid a bounds check
    if t.IndirectKey() {
     *(*unsafe.Pointer)(dst.k) = k2 // copy pointer
    } else {
     typedmemmove(t.Key, dst.k, k) // copy elem
    }
    if t.IndirectElem() {
     *(*unsafe.Pointer)(dst.e) = *(*unsafe.Pointer)(e)
    } else {
     typedmemmove(t.Elem, dst.e, e)
    }
    dst.i++
    // These updates might push these pointers past the end of the
    // key or elem arrays.  That's ok, as we have the overflow pointer
    // at the end of the bucket to protect against pointing past the
    // end of the bucket.
    dst.k = add(dst.k, uintptr(t.KeySize))
    dst.e = add(dst.e, uintptr(t.ValueSize))
   }
  }
  // Unlink the overflow buckets & clear key/elem to help GC.
  if h.flags&oldIterator == 0 && t.Bucket.PtrBytes != 0 {
   b := add(h.oldbuckets, oldbucket*uintptr(t.BucketSize))
   // Preserve b.tophash because the evacuation
   // state is maintained there.
   ptr := add(b, dataOffset)
   n := uintptr(t.BucketSize) - dataOffset
   memclrHasPointers(ptr, n)
  }
 }

 if oldbucket == h.nevacuate {
  advanceEvacuationMark(h, t, newbit)
 }
}
```

#### 参考资料

<https://qcrao.com/post/dive-into-go-map/>
