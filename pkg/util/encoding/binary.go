// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package encoding is for utilities relating to the encoding package from the stdlib
package encoding

import (
	"encoding"
)

// BinaryUnmarshalCallback returns a function that will decode the argument byte slice into *T
// using `newFn` to create an instance of *T and the encoding.BinaryUnmarshaler interface to do the actual conversion.
// `callback` will be called with the resulting *T.
// If the argument byte slice is empty, callback will be called with `nil`.
// Unmarshalling errors will be provided to the callback as the second argument.
// This function panics if `*T` does not implement encoding.BinaryUnmarshaler.
func BinaryUnmarshalCallback[T any](newFn func() *T, callback func(*T, error)) func(buf []byte) {
	// we use `any` as the type constraint rather than encoding.BinaryUnmarshaler because we are not allowed to
	// callback with `nil` in the latter case. There is a workaround, but it requires specifying two type constraints.
	// For sake of cleanliness, we resort to a runtime check here.
	if _, ok := any(new(T)).(encoding.BinaryUnmarshaler); !ok {
		panic("pointer type *T must implement encoding.BinaryUnmarshaler")
	}

	return func(buf []byte) {
		if len(buf) == 0 {
			callback(nil, nil)
			return
		}

		d := newFn()
		if err := any(d).(encoding.BinaryUnmarshaler).UnmarshalBinary(buf); err != nil {
			callback(nil, err)
			return
		}
		callback(d, nil)
	}
}
