// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package etw

import (
	"reflect"
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
#include "./c/etw.h"
*/
import "C"

// // From
// //     datadog-agent\pkg\network\tracer\common_linux.go
// //     datadog-agent\pkg\network\tracer\offsetguess.go
// func htons(a uint16) uint16 {
// 	var arr [2]byte
// 	binary.BigEndian.PutUint16(arr[:], a)
// 	return binary.LittleEndian.Uint16(arr[:])
// }

// // Should be optimized I think
// func htonla(a uint32) [4]byte {
// 	var arr [4]byte
// 	binary.BigEndian.PutUint32(arr[:], a)
// 	a2 := binary.LittleEndian.Uint32(arr[:])
// 	var arr2 [4]byte
// 	binary.LittleEndian.PutUint32(arr2[:], a2)
// 	return arr2
// }

func goBytes(data unsafe.Pointer, len C.int) []byte {
	// It could be as simple and safe as
	// 		C.GoBytes(edata, len))
	// but it copies buffer data which seems to be a waste in many
	// cases especially if it is only for a serialization. Instead
	// we make a syntetic slice which reference underlying buffer.
	// According to some measurements this approach is 10x faster
	// then built-in method

	var slice []byte
	sliceHdr := (*reflect.SliceHeader)((unsafe.Pointer(&slice)))
	sliceHdr.Cap = int(len)
	sliceHdr.Len = int(len)
	sliceHdr.Data = uintptr(data)
	return slice
}

func bytesIndexOfDoubleZero(data []byte) int {
	dataLen := len(data)
	if dataLen < 2 {
		return -1
	}

	for i := 0; i < dataLen-1; i += 2 {
		if data[i] == 0 && data[i+1] == 0 {
			return i
		}
	}

	return -1
}

// From
//    datadog-agent\pkg\util\winutil\winstrings.go
// ConvertWindowsString converts a windows c-string
// into a go string.  Even though the input is array
// of uint8, the underlying data is expected to be
// uint16 (unicode)
func convertWindowsString(winput []uint8) string {
	p := (*[1 << 29]uint16)(unsafe.Pointer(&winput[0]))[: len(winput)/2 : len(winput)/2]
	return windows.UTF16ToString(p)
}
