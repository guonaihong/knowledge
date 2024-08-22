package errgrouptest

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

// 任意一个go程出错，别的go程都退出
func Test_AllExit(t *testing.T) {
	g, ctx := errgroup.WithContext(context.TODO())

	defer g.Wait()

	// g1
	g.Go(func() error {
		fmt.Printf("run g1, %v\n", time.Now())
		<-ctx.Done()
		fmt.Printf("g1 exit:%v\n", time.Now())
		return nil
	})
	// g2
	g.Go(func() error {
		fmt.Printf("run g2, %v\n", time.Now())
		<-ctx.Done()
		fmt.Printf("g2 exit:%v\n", time.Now())
		return nil
	})
	// g3
	g.Go(func() error {
		fmt.Printf("run g3\n")
		time.Sleep(time.Second)
		return errors.New("g3 出错退出")
	})
}

func Test_Limit(t *testing.T) {
	var g errgroup.Group
	defer g.Wait()

	g.SetLimit(3)
	for _, u := range []string{
		"url1", "url2", "url3",
		"url4", "url5", "url6",
	} {
		u := u
		g.Go(func() error {
			fmt.Printf("%s, %v\n", u, time.Now())
			time.Sleep(time.Second)
			return nil
		})
	}
}

func Test_FirstErr(t *testing.T) {
	var g errgroup.Group

	defer func() {
		fmt.Println(g.Wait())
	}()

	// g1
	g.Go(func() error {
		return errors.New("g1")
	})

	// g2
	g.Go(func() error {
		return errors.New("g2")
	})
}
