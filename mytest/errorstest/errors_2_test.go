package errorstest

import (
	"errors"
	"fmt"
	"testing"
	"unsafe"
)

func Test_Errors_FmtErrorf(t *testing.T) {
	err1 := errors.New("err1")
	err2 := fmt.Errorf("err2: %w", err1)
	err3 := fmt.Errorf("err3: %w", err2)
	err4 := fmt.Errorf("err4: %w", err3)

	fmt.Println(errors.Is(err4, err1))
	fmt.Println(errors.Is(err4, err2))
	fmt.Println(errors.Is(err4, err3))
}

func Test_Errors_Join(t *testing.T) {
	err1 := errors.New("err1")
	err2 := errors.New("err2")
	joinErr := errors.Join(err1, err2)
	fmt.Println(errors.Is(joinErr, err1))
	fmt.Println(errors.Is(joinErr, err2))

}

func Join(errs ...error) error {
	e := &joinError{}
	for _, err := range errs {
		if err != nil {
			e.errs = append(e.errs, err)
		}
	}
	return e
}

type joinError struct {
	errs []error
}

func (e *joinError) Error() string {
	// Since Join returns nil if every value in errs is nil,
	// e.errs cannot be empty.
	if len(e.errs) == 1 {
		return e.errs[0].Error()
	}

	b := []byte(e.errs[0].Error())
	for _, err := range e.errs[1:] {
		b = append(b, '\n')
		b = append(b, err.Error()...)
	}
	// At this point, b has at least one byte '\n'.
	return unsafe.String(&b[0], len(b))
}

func (e *joinError) Unwrap() []error {
	return e.errs
}
func Benchmark_Errors_Join2(b *testing.B) {
	err1 := errors.New("err1")
	err2 := errors.New("err2")
	b.Run("std.join", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = errors.Join(err1, err2)
		}
	})

	b.Run("my.join", func(b *testing.B) {

		for i := 0; i < b.N; i++ {
			_ = Join(err1, err2)
		}
	})
}

func Benchmark_Errors_Find(b *testing.B) {
	err1 := errors.New("err1")
	err2 := errors.New("err2")
	err3 := errors.New("err3")
	err4 := errors.Join(err1, err2, err3)

	err11 := errors.New("err11")
	err22 := fmt.Errorf("err22: %w", err11)
	err33 := fmt.Errorf("err33: %w", err22)
	err44 := fmt.Errorf("err44: %w", err33)
	b.Run("errors.Join find err1", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			errors.Is(err4, err1)
		}
	})
	b.Run("errors.Join find err2", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			errors.Is(err4, err2)
		}
	})
	b.Run("errors.Join find err3", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			errors.Is(err4, err3)
		}
	})

	b.Run("fmt.Errorf find err11", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			errors.Is(err44, err11)
		}
	})
	b.Run("fmt.Errorf find err22", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			errors.Is(err44, err22)
		}
	})
	b.Run("fmt.Errorf find err33", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			errors.Is(err44, err33)
		}
	})
}

type ErrMsg2 struct {
	Code int
	Msg  string
}

func (e *ErrMsg2) Error() string {
	return fmt.Sprintf("code:%d, msg:%s", e.Code, e.Msg)
}

func Test_Error_As(t *testing.T) {
	err1 := fmt.Errorf("err1: %w", &ErrMsg2{Code: 100, Msg: "not found"})
	err2 := fmt.Errorf("err2: %w", err1)
	err3 := fmt.Errorf("err3: %w", err2)

	var em = &ErrMsg2{}
	if errors.As(err3, &em) {
		fmt.Printf("%#v\n", em)
	}
}
