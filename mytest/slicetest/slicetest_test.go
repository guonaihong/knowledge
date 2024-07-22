package slicetest

import (
	"fmt"
	"reflect"
	"testing"
	"unsafe"
)

func Test_ZeroSlice(t *testing.T) {
	s := make([]int, 1, 2)
	_ = s
}

func Test_EmptySlice(t *testing.T) {
	s := make([]int, 0)

	sh := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	fmt.Printf("%#v\n", sh)

	s2 := make([]int, 0)
	sh2 := (*reflect.SliceHeader)(unsafe.Pointer(&s2))
	fmt.Printf("%#v\n", sh2)
}

func Test_NilSlice(t *testing.T) {
	var s []int
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	fmt.Printf("%#v\n", sh)
}

func Test_Append(t *testing.T) {
	var s []int
	s = append(s, 1)

	s2 := make([]int, 0)
	s2 = append(s2, 1)

	fmt.Printf("s(%d:%d), s2(%d:%d)\n", len(s), cap(s), len(s2), cap(s2))
}
