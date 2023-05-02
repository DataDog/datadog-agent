// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

// Optional represents an optional type.
// By default, no value is set and a call to Get() returns (T{}, false)
type Optional[T any] struct {
	value T
	set   bool
}

// NewOptional creates a new instance of Optional[T] with a value set. A call to Get() will returns (value, true)
func NewOptional[T any](value T) Optional[T] {
	return Optional[T]{
		value: value,
		set:   true,
	}
}

// NewNoneOptional creates a new instance of Optional[T] without any value set.
func NewNoneOptional[T any]() Optional[T] {
	return Optional[T]{}
}

// Get returns the value and true if a value is set, otherwise it returns (undefined, false).
func (o *Optional[T]) Get() (T, bool) {
	return o.value, o.set
}

// Set sets a new value.
func (o *Optional[T]) Set(value T) {
	o.value = value
	o.set = true
}

// Reset removes the value set.
func (o *Optional[T]) Reset() {
	o.set = false
}

// MapOptional returns fct(value) if a value is set, otherwise it returns NewNoneOptional[T2]().
func MapOptional[T1 any, T2 any](optional Optional[T1], fct func(T1) T2) Optional[T2] {
	value, ok := optional.Get()
	if !ok {
		return NewNoneOptional[T2]()
	}
	return NewOptional(fct(value))
}
