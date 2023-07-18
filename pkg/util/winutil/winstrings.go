// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// ConvertWindowsStringList Converts a windows-style C list of strings
// (single null terminated elements
// double-null indicates the end of the list) to an array of Go strings
func ConvertWindowsStringList(winput []uint16) []string {

	if len(winput) < 2 {
		return nil
	}
	val := make([]string, 0, 5)
	from := 0

	for i := 0; i < (len(winput) - 1); i++ {
		if winput[i] == 0 {
			val = append(val, windows.UTF16ToString(winput[from:i]))
			from = i + 1

			if winput[i+1] == 0 {
				return val
			}
		}
	}
	return val

}

// ConvertWindowsString converts a windows c-string
// into a go string.  Even though the input is array
// of uint8, the underlying data is expected to be
// uint16 (unicode)
func ConvertWindowsString(winput []uint8) string {

	p := (*[1 << 29]uint16)(unsafe.Pointer(&winput[0]))[: len(winput)/2 : len(winput)/2]
	return windows.UTF16ToString(p)

}

// ConvertWindowsString16 converts a windows c-string
// into a go string.  Even though the input is array
// of uint8, the underlying data is expected to be
// uint16 (unicode)
func ConvertWindowsString16(winput []uint16) string {
	return windows.UTF16ToString(winput)
}
