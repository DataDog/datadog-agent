// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package encoding is for utilities relating to the encoding package from the stdlib
package encoding

import (
	"encoding"
)

// BinaryUnmarshalCallback returns a function that will decode the argument byte slice into T
// using `newFn` to create an instance of T and the encoding.BinaryUnmarshaler interface to do the actual conversion.
// `callback` will be called with the resulting T.
// If the argument byte slice is empty, callback will be called with `nil`.
// Unmarshalling errors will be provided to the callback as the second argument. The data argument to the callback
// may still be non-nil even if there was an error. This allows the callback to handle the allocated object, even
// in the face of errors.
func BinaryUnmarshalCallback[T encoding.BinaryUnmarshaler](newFn func() T, callback func(T, error)) func(buf []byte) {
	return func(buf []byte) {
		if len(buf) == 0 {
			var nilvalue T
			callback(nilvalue, nil)
			return
		}

		d := newFn()
		if err := d.UnmarshalBinary(buf); err != nil {
			// pass d here so callback can choose how to deal with the data
			callback(d, err)
			return
		}
		callback(d, nil)
	}
}
