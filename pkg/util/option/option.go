// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package option has optional types and functions used by Agent.
package option

// Option represents an optional type.
// By default, no value is set and a call to Get() returns (T{}, false)
type Option[T any] struct {
	value T
	set   bool
}

// New creates a new instance of Option[T] with a value set. A call to Get() will returns (value, true)
func New[T any](value T) Option[T] {
	return Option[T]{
		value: value,
		set:   true,
	}
}

// NewPtr creates a new instance of Option[T] with a value set. A call to Get() will returns (value, true)
func NewPtr[T any](value T) *Option[T] {
	option := New[T](value)
	return &option
}

// None creates a new instance of Option[T] without any value set.
func None[T any]() Option[T] {
	return Option[T]{}
}

// NonePtr creates a new instance of Option[T] without any value set.
func NonePtr[T any]() *Option[T] {
	option := None[T]()
	return &option
}

// Get returns the value and true if a value is set, otherwise it returns (undefined, false).
func (o *Option[T]) Get() (T, bool) {
	return o.value, o.set
}

// Set sets a new value.
func (o *Option[T]) Set(value T) {
	o.value = value
	o.set = true
}

// Reset removes the value set.
func (o *Option[T]) Reset() {
	o.set = false
}

// MapOption returns fct(value) if a value is set, otherwise it returns NewNoneOption[T2]().
func MapOption[T1 any, T2 any](optional Option[T1], fct func(T1) T2) Option[T2] {
	value, ok := optional.Get()
	if !ok {
		return None[T2]()
	}
	return New(fct(value))
}

// UnmarshalYAML unmarshals an Option[T] from YAML
func (o *Option[T]) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v T
	err := unmarshal(&v)
	if err != nil {
		*o = None[T]()
		return err
	}
	*o = New[T](v)
	return nil
}

// SetIfNone sets the value if it is not already set.
// Does nothing if the current instance is already set.
func (o *Option[T]) SetIfNone(value T) {
	if !o.set {
		o.Set(value)
	}
}

// SetOptionIfNone sets the option if it is not already set.
// Does nothing if the current instance is already set.
func (o *Option[T]) SetOptionIfNone(option Option[T]) {
	if !o.set {
		*o = option
	}
}
