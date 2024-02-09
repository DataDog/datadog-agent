// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package optional has optional types and functions used by Agent.
package optional

// Option represents an optional type.
// By default, no value is set and a call to Get() returns (T{}, false)
type Option[T any] struct {
	value T
	set   bool
}

// NewOption creates a new instance of Option[T] with a value set. A call to Get() will returns (value, true)
func NewOption[T any](value T) Option[T] {
	return Option[T]{
		value: value,
		set:   true,
	}
}

// NewOptionPtr creates a new instance of Option[T] with a value set. A call to Get() will returns (value, true)
func NewOptionPtr[T any](value T) *Option[T] {
	option := NewOption[T](value)
	return &option
}

// NewNoneOption creates a new instance of Option[T] without any value set.
func NewNoneOption[T any]() Option[T] {
	return Option[T]{}
}

// NewNoneOptionPtr creates a new instance of Option[T] without any value set.
func NewNoneOptionPtr[T any]() *Option[T] {
	option := NewNoneOption[T]()
	return &option
}

// IsSet returns true if a value is set.
func (o *Option[T]) IsSet() bool {
	return o.set
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
		return NewNoneOption[T2]()
	}
	return NewOption(fct(value))
}

// UnmarshalYAML unmarshals an Option[T] from YAML
func (o *Option[T]) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v T
	err := unmarshal(&v)
	if err != nil {
		*o = NewNoneOption[T]()
		return err
	}
	*o = NewOption[T](v)
	return nil
}
