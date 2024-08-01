package errorstest

import (
	"errors"
	"testing"
)

var ErrNotFound = errors.New("not found")
var ErrAlreadyExists = errors.New("already exists")

func processErr(err error) {
	if err == nil {
		// no error
		return
	}

	if err == ErrNotFound {
		// handle not found error
	} else if err == ErrAlreadyExists {
		// handle already exists error
	} else {
		// handle other errors
	}
}

func TestErrors(t *testing.T) {

}

func TestErrors_Is(t *testing.T) {
	var err error
	errors.Is(err, ErrNotFound)
}

func TestErrors_As(t *testing.T) {
	var err error
	var target error
	errors.As(err, &target)
}
