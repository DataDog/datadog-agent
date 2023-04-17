// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

// Optional reprensents an optional type.
// By default, no value is set and a call to GetValue() returns (T{}, false)
type Optional[T any] struct {
	value T
	set   bool
}

// NewOptional creates a new Optional[T] type with a value. A call to GetValue() returns (value, true)
func NewOptional[T any](value T) Optional[T] {
	return Optional[T]{
		value: value,
		set:   true,
	}
}

// GetValue() returns the value and true if NewOptional was called, otherwise it returns (T{}, false).
func (o Optional[T]) GetValue() (T, bool) {
	return o.value, o.set
}
