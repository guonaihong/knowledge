package chantest

import (
	"fmt"
	"testing"
)

func Test_Chan1(t *testing.T) {

	var c chan bool
	c <- true
}

func Test_Chan2(t *testing.T) {
	c := make(chan bool, 3)
	c <- true
	c <- true
	c <- true

	select {
	case c <- true:
	default:
	}

}

func Test_Chan3(t *testing.T) {
	c := make(chan bool)

	go func() {
		// 读
		<-c
	}()

	// 写
	c <- true
}

func Test_Chan4(t *testing.T) {

	c := make(chan bool, 4)
	c <- true
}

func Test_Chan5(t *testing.T) {
	c := make(chan bool, 5)
	c <- true
	close(c)
	// for v := range c {
	// 	fmt.Printf("%v\n", v)
	// }

exit:
	for {
		select {
		case v, ok := <-c:
			if !ok {
				break exit
			}
			fmt.Printf("%v\n", v)
		}
	}
}
