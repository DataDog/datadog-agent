// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import "unsafe"

// SliceToArray copy src bytes to dst. Destination should have enough space
func SliceToArray(src []byte, dst unsafe.Pointer) {
	//dstPtr :=
	for i := range src {
		*(*byte)(unsafe.Pointer(uintptr(dst) + uintptr(i))) = src[i]
		//dstPtr++
	}
}
