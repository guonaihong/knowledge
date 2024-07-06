package main

import (
	"flag"
	"sync"
)

func main() {
	// 开起几个go程
	n := flag.Int("n", 0, "number of go routines")
	flag.Parse()
	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Add(*n)
	for i := 0; i < *n; i++ {
		go func() {
			for {
				// 无限循环
			}
		}()
	}
}
