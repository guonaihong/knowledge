package second

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/ratelimit"
	"golang.org/x/time/rate"
)

func Test_Example(t *testing.T) {
	rl := ratelimit.New(10, ratelimit.WithSlack(3)) // per second

	prev := time.Now()
	for i := 0; i < 10; i++ {
		now := rl.Take()
		if i == 0 {
			time.Sleep(time.Second / 2.0)
		}
		fmt.Println(i, now.Sub(prev))
		fmt.Printf("counter: %v, %v \n", i, time.Now().Format(time.RFC3339Nano))
		prev = now
	}

}

func Test_Example3(t *testing.T) {

}

// 範例程式碼：https://www.jianshu.com/p/1ecb513f7632
func Test_Example2(t *testing.T) {
	counter := 0
	ctx := context.Background()

	// 每 200 毫秒會放一次 token 到桶子（每秒會放 5 個 token 到桶子），bucket 最多容納 1 個 token
	limit := rate.Every(time.Millisecond * 500)
	limiter := rate.NewLimiter(limit, 3)
	time.Sleep(time.Second)
	fmt.Println(limiter.Limit(), limiter.Burst()) // 5，1

	for i := 0; i < 10; i++ {
		counter++
		limiter.Wait(ctx)
		fmt.Printf("counter: %v, %v \n", counter, time.Now().Format(time.RFC3339Nano))
	}
}
