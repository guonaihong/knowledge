package slicetest

import "testing"

func Benchmark_Slice1(b *testing.B) {

	for i := 0; i < b.N; i++ {
		s := make([]int, 0)
		_ = s
	}
}

func Benchmark_Slice2(b *testing.B) {

	for i := 0; i < b.N; i++ {
		var s []int
		_ = s
	}
}
