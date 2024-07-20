package second

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func Test_Context(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.TODO())
	_ = ctx
	cancel(errors.New("test"))
}

func Test_Context2(t *testing.T) {
	ctx := context.TODO()
	ctx2 := context.WithValue(ctx, "111", "1111")
	ctx22 := context.WithValue(ctx, "22", "222")
	_ = ctx22
	ctx3 := context.WithValue(ctx2, "333", "3333")

	fmt.Println(ctx3.Value("22"))

}
