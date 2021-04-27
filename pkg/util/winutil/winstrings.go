// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build windows

package winutil

import (
	"unicode/utf16"
	"unsafe"
)

// ConvertWindowsStringList Converts a windows-style C list of strings
// (single null terminated elements
// double-null indicates the end of the list) to an array of Go strings
func ConvertWindowsStringList(winput []uint16) []string {

	if len(winput) < 2 {
		return []string{}
	}
	if winput[len(winput)-1] == 0 {
		winput = winput[:len(winput)-1] // remove terminating null
	}
	val := make([]string, 0, 5)
	from := 0
	for i, c := range winput {
		if c == 0 {
			val = append(val, string(utf16.Decode(winput[from:i])))
			from = i + 1
		}
	}
	return val

}

// ConvertWindowsString converts a windows c-string
// into a go string.  Even though the input is array
// of uint8, the underlying data is expected to be
// uint16 (unicode)
func ConvertWindowsString(winput []uint8) string {

	if len(winput) < 2 {
		return ""
	}
	if winput[len(winput)-1] == 0 {
		winput = winput[:len(winput)-1] // remove terminating null
	}

	p := (*[1 << 29]uint16)(unsafe.Pointer(&winput[0]))[: len(winput)/2 : len(winput)/2]
	return string(utf16.Decode(p))

}

// ConvertWindowsString16 converts a windows c-string
// into a go string.  Even though the input is array
// of uint8, the underlying data is expected to be
// uint16 (unicode)
func ConvertWindowsString16(winput []uint16) string {
	if len(winput) < 2 {
		return ""
	}
	if winput[len(winput)-1] == 0 {
		winput = winput[:len(winput)-1] // remove terminating null
	}

	return string(utf16.Decode(winput))
}

// ConvertASCIIString converts a c-string into
// a go string
func ConvertASCIIString(input []byte) string {

	return string(input)
}
