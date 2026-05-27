// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpfcheck

import (
	"math"
	"math/bits"
	"os"
)

// integer is the set of all integer types, mirroring constraints.Integer from golang.org/x/exp.
type integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

var (
	pageSize  = os.Getpagesize()
	pageShift = uint(math.Log2(float64(pageSize)))
)

// round x up to the nearest power of y
func roundUp[T integer](x, y T) T {
	return ((x + (y - 1)) / y) * y
}

// round x up to the nearest power of y, where y is a power of 2
func roundUpPow2[T integer](x, y T) T {
	return ((x - 1) | (y - 1)) + 1
}

// round x up to the nearest power of 2
func roundUpNearestPow2(x uint32) uint32 {
	return uint32(1) << bits.Len32(x-1)
}

func pageAlign[T integer](x T) T {
	return align(x, T(pageSize))
}

func align[T integer](x, a T) T {
	return (x + (a - 1)) & ^(a - 1)
}
