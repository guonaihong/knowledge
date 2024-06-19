package second

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/ratelimit"
	"golang.org/x/time/rate"
)

func main2() {
	rl := ratelimit.New(10) // per second

	time.Sleep(time.Second / 2.0)
	prev := time.Now()
	for i := 0; i < 10; i++ {
		now := rl.Take()
		fmt.Println(i, now.Sub(prev))
		prev = now
	}

}

// 範例程式碼：https://www.jianshu.com/p/1ecb513f7632
func main() {
	counter := 0
	ctx := context.Background()

	// 每 200 毫秒會放一次 token 到桶子（每秒會放 5 個 token 到桶子），bucket 最多容納 1 個 token
	limit := rate.Every(time.Millisecond * 200)
	limiter := rate.NewLimiter(limit, 1)
	fmt.Println(limiter.Limit(), limiter.Burst()) // 5，1

	for {
		counter++
		limiter.Wait(ctx)
		fmt.Printf("counter: %v, %v \n", counter, time.Now().Format(time.RFC3339))
	}
}
