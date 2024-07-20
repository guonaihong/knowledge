package chantest

import (
	"fmt"
	"testing"
)

// 一、写空chan

// 写空chan分为两种情况
// 1. panic
// package main

//	func main() {
//		var c chan bool
//		c <- true
//	}

// 输出是这样的
// fatal error: all goroutines are asleep - deadlock!
// goroutine 1 [chan send (nil chan)]:

// 2. 阻塞
func Test_ChanWrite1(t *testing.T) {
	var c chan bool
	c <- true
}

// 二、写关闭的chan
// 结果是panic
func Test_ChanWrite2(t *testing.T) {
	c := make(chan bool, 3)
	close(c)
	c <- true

}

// 三、写入一个有空间的，可以成功写入
func Test_ChanWrite3(t *testing.T) {
	t.Run("block write", func(t *testing.T) {
		// 3.1 阻塞写
		c := make(chan bool, 3)
		c <- true
		fmt.Printf("写入成功, len:%d, success = %t\n", len(c), len(c) == 1)

	})

	t.Run("non-block write", func(t *testing.T) {
		// 3.2 非阻塞写
		c := make(chan bool, 3)
		select {
		case c <- true:
		default:
		}

		fmt.Printf("写入成功, len:%d, success = %t\n", len(c), len(c) == 2)
	})
}

// 第四种情况, 已经有读go程等待
func Test_ChanWrite4(t *testing.T) {
	t.Run("block write, there is already a go process waiting", func(t *testing.T) {
		// 4.1 阻塞写, 已经有读go程等待
		c := make(chan bool, 3)
		go func() {
			// 读go程
			<-c
			fmt.Printf("4.1读取成功\n")
		}()
		c <- true

	})

	t.Run("non-block write, there is already a go process waiting", func(t *testing.T) {
		// 4.2 非阻塞写, 已经有读go程等待
		c := make(chan bool, 3)
		go func() {
			// 读go程
			<-c
			fmt.Printf("4.2读取成功\n")
		}()
		select {
		case c <- true:
		default:
		}
	})
}

// 第五种情况， 对非缓存的chan写入，这时候没有read go程等待的情况
func Test_ChanWrite5(t *testing.T) {
	// 有空间在第三种情况讨论过，现在是非缓存的chan
	t.Run("5.2 non-block write", func(t *testing.T) {
		// 5.2 非阻塞写
		c := make(chan bool)
		select {
		case c <- true:
		default:
		}
		fmt.Printf("5.2写入成功\n")
	})
	t.Run("block write", func(t *testing.T) {
		// 5.1 阻塞写, 已经有读go程等待
		c := make(chan bool)

		c <- true
		fmt.Printf("5.1写入成功\n")
	})

}
