package second

import (
	"context"
	"errors"
	"testing"
)

func Test_Context(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.TODO())
	_ = ctx
	cancel(errors.New("test"))
}
