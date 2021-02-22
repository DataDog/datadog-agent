// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"unsafe"

	"github.com/pkg/errors"
)

var (
	// ErrStringArrayOverflow returned when there is a string array overflow
	ErrStringArrayOverflow = errors.New("string array overflow")
)

// SliceToArray copy src bytes to dst. Destination should have enough space
func SliceToArray(src []byte, dst unsafe.Pointer) {
	//dstPtr :=
	for i := range src {
		*(*byte)(unsafe.Pointer(uintptr(dst) + uintptr(i))) = src[i]
		//dstPtr++
	}
}

// UnmarshalStringArray extract array of string for array of byte
func UnmarshalStringArray(data []byte) ([]string, error) {
	var result []string

	for i := uint32(0); int(i) < len(data); {
		// size of arg
		n := ByteOrder.Uint32(data[i : i+4])
		if n <= 0 {
			return result, nil
		}
		i += 4

		if int(i+n) > len(data) {
			return result, ErrStringArrayOverflow
		}

		arg := string(data[i : i+n])
		i += n

		result = append(result, arg)
	}

	return result, ErrStringArrayOverflow
}
