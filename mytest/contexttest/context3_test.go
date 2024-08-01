package contexttest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func Test_Contex3_tree(t *testing.T) {
	ctx1, cancel := context.WithCancel(context.TODO())

	ctx2, _ := context.WithCancel(ctx1)
	ctx3, _ := context.WithCancel(ctx1)

	_ = ctx2
	_ = ctx3
	_ = cancel
}

func Test_Contex3_tree2(t *testing.T) {
	ctx1, cancel := context.WithCancel(context.TODO())

	ctx2, _ := context.WithCancel(ctx1)
	ctx3, _ := context.WithCancel(ctx1)

	cancel()
	select {
	case <-ctx2.Done():
		fmt.Printf("ctx2 done\n")
	default:
	}
	select {
	case <-ctx3.Done():
		fmt.Printf("ctx3 done\n")
	default:
	}
}

func Test_Context3_tree3(t *testing.T) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	defer wg.Wait()

	go func() {
		<-done
		fmt.Printf("A done\n")
		defer wg.Done()
	}()

	go func() {
		<-done
		fmt.Printf("B done\n")
		defer wg.Done()
	}()
	close(done)
}

type myContext struct {
	c context.Context
}

// 实现context.Context接口
func (c *myContext) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

func (c *myContext) Done() <-chan struct{} {
	return c.c.Done()
}

func (c *myContext) Err() error {
	return nil
}

func (c *myContext) Value(key interface{}) interface{} {
	return nil
}

func Test_Contex3_2(t *testing.T) {

	ctx, cancel := context.WithCancelCause(context.TODO())

	ctx2, _ := context.WithCancel(&myContext{c: ctx})
	ctx3, _ := context.WithCancel(ctx2)
	_ = ctx2
	_ = ctx3
	cancel(nil)
}
