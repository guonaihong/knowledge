package errorstest

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
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
	// var err error
	// var target error
	// errors.As(err, &target)
}

func TestError_Join(t *testing.T) {
	var err error
	errors.Join(err, ErrNotFound)
}

func Test_Errors_2019_Before(t *testing.T) {
	_, err := http.Get("http://127.0.0.1:33333")
	if uerr, ok := err.(*url.Error); ok {
		if noerr, ok := uerr.Err.(*net.OpError); ok {
			if scerr, ok := noerr.Err.(*os.SyscallError); ok {
				if scerr.Err == syscall.ECONNREFUSED {
					fmt.Printf("xxx: (7) couldn't connect to host\n")
					return
				}
			}
		}
	}
}

func Test_Errors_2019(t *testing.T) {
	_, err := http.Get("http://127.0.0.1:33333")
	if errors.Is(err, syscall.ECONNREFUSED) {
		fmt.Printf("xxx: (7) couldn't connect to host\n")
		return
	}
}

func fetchURL_2019_Before(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response code: %v", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return body, nil
}

func Test_Errors_2019_Before_Wrap(t *testing.T) {
	_, err := fetchURL_2019_Before("http://127.0.0.1:33333")
	switch {
	case strings.Contains(err.Error(), "failed to fetch URL"):
		fmt.Println(err)
	case strings.Contains(err.Error(), "received non-200 response code"):
		fmt.Println(err)
	case strings.Contains(err.Error(), "failed to read response body"):
		fmt.Println(err)
	}
}

var (
	ErrNon200Response = errors.New("received non-200 response code")
	ErrHTTPGet        = errors.New("failed to fetch URL")
	ErrReadBody       = errors.New("failed to read response body")
)

func fetchURL_2019(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("%w:%w", ErrHTTPGet, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %v", ErrNon200Response, resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReadBody, err)
	}

	return body, nil
}

func Test_Errors_2019_Wrap(t *testing.T) {
	_, err := fetchURL_2019("http://127.0.0.1:33333")
	switch {
	case errors.Is(err, ErrHTTPGet):
		fmt.Println(err)
	case errors.Is(err, ErrNon200Response):
		fmt.Println(err)
	case errors.Is(err, ErrReadBody):
		fmt.Println(err)
	}
}

func Test_Errors_2019_Wrap2(t *testing.T) {

	var err1 = errors.New("err1")
	var err2 = errors.New("err2")

	var err3 = fmt.Errorf("%w%w", err1, err2)

	fmt.Println(errors.Is(err3, err1))
	fmt.Println(errors.Is(err3, err2))
}

var (
	ErrCodeMsgNotFound      error = &ErrMsg{404, "not found"}
	ErrCodeMsgAlreadyExists error = &ErrMsg{409, "already exists"}
)

type ErrMsg struct {
	Code int
	Msg  string
}

func (e ErrMsg) Error() string {
	return fmt.Sprintf("code:%d, msg:%s", e.Code, e.Msg)
}
func processErrCodeMsg(err error) {
	if err == nil {
		return
	}
	var em = &ErrMsg{}
	if errors.As(err, &em) {
		fmt.Printf("err code: %d, msg: %s\n", em.Code, em.Msg)
	}

}
func rpcCall() error {
	rpcRedisFindUser := func() error {
		return fmt.Errorf("rpc redis error")
	}

	if err := rpcRedisFindUser(); err != nil {
		return fmt.Errorf("%w:%w", err, ErrCodeMsgNotFound)
	}

	return nil
}
func Test_ErrMsg(t *testing.T) {

	processErrCodeMsg(rpcCall())
}

func Test_Errors_2023_Wrap2(t *testing.T) {

	var err1 = errors.New("err1")
	var err2 = errors.New("err2")

	var err3 = errors.Join(err1, err2)

	fmt.Println(errors.Is(err3, err1))
	fmt.Println(errors.Is(err3, err2))
}

func Benchmark_Errors_Join(b *testing.B) {

	for i := 0; i < b.N; i++ {

		var err1 = errors.New("err1")
		var err2 = errors.New("err2")
		_ = errors.Join(err1, err2)
	}

}

func Benchmark_Errors_Wrap(b *testing.B) {

	for i := 0; i < b.N; i++ {

		var err1 = errors.New("err1")
		var err2 = errors.New("err2")
		_ = fmt.Errorf("%w:%w", err1, err2)
	}

}
