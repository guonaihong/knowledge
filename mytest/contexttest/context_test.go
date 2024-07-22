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
	ctx1 := context.WithValue(ctx, "1", "1")
	ctx2 := context.WithValue(ctx1, "2", "2")
	ctx3 := context.WithValue(ctx2, "3", "3")

	val := ctx3.Value("22")
	fmt.Println(val)

}
