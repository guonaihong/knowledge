package waitgroup

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func Test_Add(t *testing.T) {
	t.Run("Add", func(t *testing.T) {
		total := 0
		var wg sync.WaitGroup
		wg.Add(3)
		defer func() {
			wg.Wait()
			fmt.Printf("raw.total:%d\n", total)
		}()
		for i := 0; i < 3; i++ {
			go func() {
				defer wg.Done()

				for i := 0; i < 10000; i++ {
					total++
				}
			}()
		}
	})

	t.Run("mutex-Add", func(t *testing.T) {
		total := 0
		var wg sync.WaitGroup
		wg.Add(3)
		defer func() {
			wg.Wait()
			fmt.Printf("mutext.total:%d\n", total)
		}()
		var mu sync.Mutex
		for i := 0; i < 3; i++ {
			go func() {
				defer wg.Done()

				for i := 0; i < 10000; i++ {

					mu.Lock()
					total++
					mu.Unlock()
				}
			}()
		}
	})

	t.Run("atomic-Add", func(t *testing.T) {
		total := int64(0)
		var wg sync.WaitGroup
		wg.Add(3)
		defer func() {
			wg.Wait()
			fmt.Printf("atomic.total:%d\n", total)
		}()
		for i := 0; i < 3; i++ {
			go func() {
				defer wg.Done()

				for i := 0; i < 10000; i++ {

					atomic.AddInt64(&total, 1)
				}
			}()
		}
	})
}

func Test_Cas(t *testing.T) {

	i32 := int32(0)
	atomic.CompareAndSwapInt32(&i32, 0, 1)
	fmt.Printf("%d\n", i32)

	if i32 == 0 {
		i32 = 1
	}
}
