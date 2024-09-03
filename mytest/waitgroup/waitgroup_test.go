package waitgroup

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

type call struct {
	g      sync.WaitGroup
	result any
	err    error
}

type Resource struct {
	mu sync.Mutex
	m  map[string]*call
}

func (r *Resource) FristResource(key string, callback func() (any, error)) (val any, err error) {
	r.mu.Lock()
	c, ok := r.m[key]
	if !ok {
		if r.m == nil {
			r.m = make(map[string]*call)
		}

		c = &call{}
		r.m[key] = c
		c.g.Add(1)
		r.mu.Unlock()

		c.result, c.err = callback()

		r.mu.Lock()
		delete(r.m, key)
		c.g.Done()
		r.mu.Unlock()

		return c.result, c.err
	}

	r.mu.Unlock()
	c.g.Wait()
	return c.result, c.err
}

func Test_Frist(t *testing.T) {
	var r Resource
	var wg sync.WaitGroup
	wg.Add(2)
	defer wg.Wait()

	go func() {
		defer wg.Done()

		val, err := r.FristResource("key", func() (any, error) {
			time.Sleep(time.Second * 2)
			return fmt.Sprintf("g1.key:%v", time.Now()), nil
		})
		fmt.Println(val, err)
	}()
	go func() {
		defer wg.Done()
		val, err := r.FristResource("key", func() (any, error) {
			time.Sleep(time.Second * 2)
			return fmt.Sprintf("g2.key:%v", time.Now()), nil
		})
		fmt.Println(val, err)
	}()
}

func Test_Wait(t *testing.T) {
	var wg sync.WaitGroup

	wg.Add(1)
	// main
	defer func() {
		wg.Wait()
		fmt.Printf("g1.done:%v\n", time.Now())
	}()

	// g1
	go func() {
		wg.Wait()
		fmt.Printf("g2.done:%v\n", time.Now())
	}()

	// g2
	go func() {
		defer wg.Done()
		time.Sleep(time.Second * 2)
	}()
}

func Test_Example_WaitGroup(t *testing.T) {

	var wg sync.WaitGroup
	wg.Add(2)
	defer wg.Wait()
	go func() {
		defer wg.Done()
		time.Sleep(time.Second)
		fmt.Printf("done 1:%v\n", time.Now())
	}()

	go func() {
		defer wg.Done()
		time.Sleep(time.Second)
		fmt.Printf("done 1:%v\n", time.Now())
	}()
}

func Test_WaitGroup_Easy(t *testing.T) {

	var wg sync.WaitGroup
	wg.Add(2)
	defer wg.Wait()

	go func() {
		defer wg.Done()
		time.Sleep(time.Second)
		fmt.Printf("done 1:%v\n", time.Now())
	}()

	go func() {
		defer wg.Done()
		time.Sleep(time.Second)
		fmt.Printf("done 1:%v\n", time.Now())
	}()
}
