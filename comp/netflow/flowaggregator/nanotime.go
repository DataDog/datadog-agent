// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package flowaggregator

import (
	_ "unsafe" // required for go:linkname
)

//go:linkname runtimeNanoTime runtime.nanotime
func runtimeNanoTime() int64

// Now returns monotonic time in nanoseconds since an unspecified start point.
func nanoNow() int64 {
	return runtimeNanoTime()
}

// Since returns the duration in nanoseconds since the given start time (in ns).
func nanoSince(start int64) int64 {
	return nanoNow() - start
}
