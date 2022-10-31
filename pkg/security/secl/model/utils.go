// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"regexp"
)

// containerIDPattern is the pattern of a container ID
var containerIDPattern = regexp.MustCompile(fmt.Sprintf(`([[:xdigit:]]{%v})`, sha256.Size*2))

// FindContainerID extracts the first sub string that matches the pattern of a container ID
func FindContainerID(s string) string {
	return containerIDPattern.FindString(s)
}

// SliceToArray copy src bytes to dst. Destination should have enough space
func SliceToArray(src []byte, dst []byte) {
	if len(src) != len(dst) {
		panic("different len in SliceToArray")
	}

	copy(dst, src)
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
			arg := NullTerminatedString(data[i:len])
			return append(result, arg), ErrStringArrayOverflow
		}

		arg := NullTerminatedString(data[i : i+n])
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

	return NullTerminatedString(data[:size]), nil
}

func NullTerminatedString(d []byte) string {
	idx := bytes.IndexByte(d, 0)
	if idx == -1 {
		return string(d)
	}
	return string(d[:idx])
}

// UnmarshalPrintableString unmarshal printable string
func UnmarshalPrintableString(data []byte, size int) (string, error) {
	if len(data) < size {
		return "", ErrNotEnoughData
	}

	str, err := UnmarshalString(data, size)
	if err != nil {
		return "", err
	}
	if !IsPrintable(str) {
		return "", ErrNonPrintable
	}

	return str, nil
}
