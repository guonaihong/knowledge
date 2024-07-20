package chantest

import (
	"fmt"
	"testing"
)

// 第一种情况：读空chan
// 1.1 读空chan，panic

// package main
// func main() {
// 	c := make(chan bool, 1)
// 	<-c
// }

// 输出结果
// fatal error: all goroutines are asleep - deadlock!

// 1.2 读空chan，阻塞
func Test_ChanRead1(t *testing.T) {
	c := make(chan bool, 1)
	<-c
}

// 第二种情况: 读关闭的chan，直接返回，这个特性也是构成context包的基础
func Test_ChanRead2(t *testing.T) {
	c := make(chan bool, 1)
	close(c)
	<-c
	fmt.Printf("case 2.成功返回\n")
}

// 第三种情况: 读有数据的chan
func Test_ChanRead3(t *testing.T) {
	t.Run("read block", func(t *testing.T) {

		c := make(chan bool, 1)
		c <- true
		// 读取有数据的chan

		<-c
		fmt.Printf("case 3.1 读取成功\n")
	})

	t.Run("read non-block", func(t *testing.T) {
		c := make(chan bool, 1)
		c <- true
		select {
		case <-c:
		default:
		}
		fmt.Printf("case 3.2 读取成功\n")
	})
}

// 第四种情况: 读取数据时，有阻塞的write go程
func Test_ChanRead4(t *testing.T) {

	t.Run("read block", func(t *testing.T) {
		c := make(chan bool, 1)
		go func() {
			// 写
			c <- true
		}()

		// 读
		<-c
		fmt.Printf("case 4.1 读取成功 %t\n", len(c) == 0)
	})

	t.Run("read non-block", func(t *testing.T) {
		c := make(chan bool, 1)
		go func() {
			// 写
			c <- true
		}()

		select {
		case <-c:
		default:
		}
		fmt.Printf("case 4.2 读取成功 %t\n", len(c) == 0)
	})
}

// 第五种情况: 读取数据时，没有write go程
func Test_ChanRead5(t *testing.T) {

	t.Run("read non-block", func(t *testing.T) {
		c := make(chan bool)

		select {
		case <-c:
		default:
		}
		fmt.Printf("case 5.2 读取成功\n")
	})
	t.Run("read block", func(t *testing.T) {
		c := make(chan bool)
		// 读
		<-c
		fmt.Printf("case 5.1 读取成功\n")
	})
}
