package contexttest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func Test_Context(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.TODO())
	_ = ctx
	cancel(errors.New("test"))
}

func Test_Context2(t *testing.T) {
	ctx := context.TODO()
	ctx1 := context.WithValue(ctx, "1", "1")
	ctx2 := context.WithValue(ctx1, "2", "2")
	ctx3 := context.WithValue(ctx2, "3", "3")

	val := ctx3.Value("2")
	fmt.Println(val)

}

// context(上)用到
func Test_Detach(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ctx1 := context.WithValue(ctx, "traceID", "traceID-value")
	ctx2 := context.WithValue(ctx1, "traceID-2", "traceID-value2")

	defer cancel()
	// 模拟向消息服务发送消息
	go func() {
		// rpcCall里面只要想持有父context用于打通链路id, 方便排查问题
		rpcCall := func() {
			select {
			case <-ctx2.Done():
				fmt.Printf("rpc call canceled\n")
				return
			default:
			}
			// 模拟发送消息
			fmt.Printf("send ok\n")
		}
		rpcCall()
	}()
}

// context(中) 用到
func Test_Detach_New2(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ctx1 := context.WithValue(ctx, "traceID", "traceID-value")
	ctx2 := context.WithValue(ctx1, "traceID-2", "traceID-value2")

	// 模拟主go程退出的情况
	cancel()
	// 新加的这行代码
	newCtx := context.WithoutCancel(ctx2)
	// 模拟向消息服务发送消息
	go func() {
		// rpcCall里面只要想持有父context用于打通链路id, 方便排查问题
		rpcCall := func() {
			select {
			case <-newCtx.Done():
				fmt.Printf("rpc call canceled\n")
				return
			default:
			}
			// 模拟发送消息
			fmt.Printf("send ok:%s\n", ctx2.Value("traceID-2"))
			fmt.Printf("send ok:%s\n", ctx2.Value("traceID"))
		}
		rpcCall()
	}()
}

func Test_Detach_New(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ctx1 := context.WithValue(ctx, "traceID", "traceID-value")
	ctx2 := context.WithValue(ctx1, "traceID-2", "traceID-value2")

	defer cancel()
	// 这加的这行代码
	newCtx := context.WithoutCancel(ctx2)
	// 模拟向消息服务发送消息
	go func() {
		// rpcCall里面只要想持有父context用于打通链路id, 方便排查问题
		rpcCall := func() {
			select {
			case <-newCtx.Done():
				fmt.Printf("rpc call canceled\n")
				return
			default:
			}
			// 模拟发送消息
			fmt.Printf("send ok\n")
		}
		rpcCall()
	}()
}

func myCancel(once *sync.Once, done chan struct{}) {
	once.Do(func() {
		close(done)
	})
}
func Test_Cancel(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(2)
	defer wg.Wait()
	var once sync.Once

	done := make(chan struct{})
	go func() {
		defer wg.Done()

		select {
		case <-done:
		}

		// 假如业务出错
		//myCancel(&once, done)
	}()

	go func() {
		defer wg.Done()
		select {
		case <-done:
		}
		// 假如业务出错
		// myCancel(&once, done)
	}()
	myCancel(&once, done)
}

func Test_Cancel2(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(2)
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.TODO())
	go func() {
		defer wg.Done()

		select {
		case <-ctx.Done():
		}

		// 假如业务出错
		//myCancel(&once, done)
	}()

	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
		}
		// 假如业务出错
		// myCancel(&once, done)
	}()
	cancel()
}
