```go
// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package errors

import (
	"internal/reflectlite"
)

// Unwrap 函数返回通过调用 err 的 Unwrap 方法得到的结果，如果 err 的类型包含一个返回 error 的 Unwrap 方法。
// 否则，Unwrap 返回 nil。
//
// Unwrap 只调用形式为 "Unwrap() error" 的方法。
// 特别是 Unwrap 不会解包由 [Join] 返回的错误。
func Unwrap(err error) error {
	u, ok := err.(interface {
		Unwrap() error
	})
	if !ok {
		return nil
	}
	return u.Unwrap()
}

// Is 报告 err 的树中的任何错误是否与 target 匹配。
//
// 树由 err 本身组成，然后是通过重复调用其 Unwrap() error 或 Unwrap() []error 方法获得的错误。
// 当 err 包装多个错误时，Is 检查 err 后跟其子节点的深度优先遍历。
//
// 如果错误等于目标或实现了 Is(error) bool 方法，使得 Is(target) 返回 true，则认为错误与目标匹配。
//
// 错误类型可能提供一个 Is 方法，以便它可以被视为与现有错误等效。例如，如果 MyError 定义了
//
//	func (m MyError) Is(target error) bool { return target == fs.ErrExist }
//
// 那么 Is(MyError{}, fs.ErrExist) 返回 true。参见 [syscall.Errno.Is] 在标准库中的示例。
// Is 方法应该只浅比较 err 和目标，而不调用 [Unwrap] 在任一上。
func Is(err, target error) bool {
	if target == nil {
		return err == target
	}

	isComparable := reflectlite.TypeOf(target).Comparable()
	return is(err, target, isComparable)
}

func is(err, target error, targetComparable bool) bool {
	for {
		if targetComparable && err == target {
			return true
		}
		if x, ok := err.(interface{ Is(error) bool }); ok && x.Is(target) {
			return true
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
			if err == nil {
				return false
			}
		case interface{ Unwrap() []error }:
			for _, err := range x.Unwrap() {
				if is(err, target, targetComparable) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}
}

// As 在 err 的树中找到第一个与 target 匹配的错误，如果找到一个，则将 target 设置为该错误值并返回 true。
// 否则，它返回 false。
//
// 树由 err 本身组成，然后是通过重复调用其 Unwrap() error 或 Unwrap() []error 方法获得的错误。
// 当 err 包装多个错误时，As 检查 err 后跟其子节点的深度优先遍历。
//
// 如果错误的具体值可分配给 target 指向的值，或者错误有一个方法 As(interface{}) bool 使得 As(target) 返回 true，则错误匹配 target。
// 在后一种情况下，As 方法负责设置 target。
//
// 错误类型可能提供一个 As 方法，以便它可以被视为不同的错误类型。
//
// As 如果 target 不是指向实现 error 的类型或任何接口类型的非 nil 指针，则会 panic。
func As(err error, target any) bool {
	if err == nil {
		return false
	}
	if target == nil {
		panic("errors: target cannot be nil")
	}
	val := reflectlite.ValueOf(target)
	typ := val.Type()
	if typ.Kind() != reflectlite.Ptr || val.IsNil() {
		panic("errors: target must be a non-nil pointer")
	}
	targetType := typ.Elem()
	if targetType.Kind() != reflectlite.Interface && !targetType.Implements(errorType) {
		panic("errors: *target must be interface or implement error")
	}
	return as(err, target, val, targetType)
}

func as(err error, target any, targetVal reflectlite.Value, targetType reflectlite.Type) bool {
	for {
		if reflectlite.TypeOf(err).AssignableTo(targetType) {
			targetVal.Elem().Set(reflectlite.ValueOf(err))
			return true
		}
		if x, ok := err.(interface{ As(any) bool }); ok && x.As(target) {
			return true
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
			if err == nil {
				return false
			}
		case interface{ Unwrap() []error }:
			for _, err := range x.Unwrap() {
				if err == nil {
					continue
				}
				if as(err, target, targetVal, targetType) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}
}

var errorType = reflectlite.TypeOf((*error)(nil)).Elem()
```