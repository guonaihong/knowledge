package main

import (
	"fmt"
	"runtime"
	"time"
)

func main() {
	var memStats runtime.MemStats

	var all = map[int][]byte{}
	for j := 0; ; j++ {
		// Allocate a large block of memory
		bigSlice := make([]byte, 1024*1024*10) // 1 MB
		for i := range bigSlice {
			bigSlice[i] = 1
		}
		all[j] = bigSlice

		// Print memory statistics
		runtime.ReadMemStats(&memStats)
		fmt.Printf("Alloc = %v MiB\n", memStats.Alloc/1024/1024)
		fmt.Printf("TotalAlloc = %v MiB\n", memStats.TotalAlloc/1024/1024)
		fmt.Printf("Sys = %v MiB\n", memStats.Sys/1024/1024)
		fmt.Printf("NumGC = %v\n", memStats.NumGC)

		// Sleep for a while before allocating more memory
		time.Sleep(1 * time.Second)
	}
}
