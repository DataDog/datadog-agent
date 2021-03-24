// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"bytes"
	"unsafe"
)

// SliceToArray copy src bytes to dst. Destination should have enough space
func SliceToArray(src []byte, dst unsafe.Pointer) {
	for i := range src {
		*(*byte)(unsafe.Pointer(uintptr(dst) + uintptr(i))) = src[i]
	}
}

// UnmarshalStringArray extract array of string for array of byte
func UnmarshalStringArray(data []byte) ([]string, error) {
	var result []string
	len := uint32(len(data))

	for i := uint32(0); i < len; {
		if i+4 >= len {
			return result, ErrStringArrayOverflow
		}
		// size of arg
		n := ByteOrder.Uint32(data[i : i+4])
		if n == 0 {
			return result, nil
		}
		i += 4

		if i+n > len {
			// truncated
			arg := string(bytes.Trim(data[i:len-1], "\x00"))
			return append(result, arg), ErrStringArrayOverflow
		}

		arg := string(bytes.Trim(data[i:i+n], "\x00"))
		i += n

		result = append(result, arg)
	}

	return result, nil
}

// UnmarshalString unmarshal string
func UnmarshalString(data []byte, size int) (string, error) {
	if len(data) < size {
		return "", ErrNotEnoughData
	}

	str := string(bytes.Trim(data[0:size], "\x00"))
	if !IsPrintable(str) {
		return "", ErrNonPrintable
	}

	return str, nil
}
