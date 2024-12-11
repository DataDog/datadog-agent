// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import (
	"errors"
)

// Value represents either an error or an actual value of type T.
//
// The default value of the type is an error saying that the value was not initialized.
type Value[T any] struct {
	value       T
	err         error
	initialized bool
}

// NewValue initializes a Value[T] with the given value of type T and no error.
func NewValue[T any](value T) Value[T] {
	return Value[T]{
		value:       value,
		initialized: true,
	}
}

// NewValueFrom returns a Value[T] from a value and an error.
// If the error is non-nil then it represents this error, otherwise it represents the value.
//
// This is a convenient function to get a Value from a function which returns a value and an error.
func NewValueFrom[T any](value T, err error) Value[T] {
	if err != nil {
		return NewErrorValue[T](err)
	}
	return NewValue(value)
}

// NewErrorValue initializes a Value[T] with the given error.
//
// Note that if err is nil, the returned Value[T] is fundamentally equivalent to a Value[T]
// containing the default value of T and no error.
func NewErrorValue[T any](err error) Value[T] {
	return Value[T]{
		err:         err,
		initialized: true,
	}
}

// Value returns the value and the error stored in the Value[T].
//
// If it contains an error, the returned value is unspecified.
func (value *Value[T]) Value() (T, error) {
	if value.initialized {
		return value.value, value.err
	}

	var def T
	return def, errors.New("value not initialized")
}

// Error returns the error stored in the Value[T], or nil if it doesn't contain an error.
func (value *Value[T]) Error() error {
	_, err := value.Value()
	return err
}

// ValueOrDefault returns the value stored in the Value[T], or the default value of the type
// in case of error.
func (value *Value[T]) ValueOrDefault() T {
	val, err := value.Value()
	if err == nil {
		return val
	}

	var def T
	return def
}
