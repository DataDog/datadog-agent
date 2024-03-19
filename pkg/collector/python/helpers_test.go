// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"strings"
	"testing"
)

var stringsSink []string

func Benchmark_cStringArrayToSlice(b *testing.B) {
	const numStrings = 10 // fermi estimate
	const stringLen = 10  // fermi estimate
	slice := make([]string, numStrings)
	for i := range slice {
		slice[i] = strings.Repeat("a", stringLen)
	}
	// Note: cArray is not freed, but that's fine for the benchmark.
	cArray := testHelperSliceToCStringArray(slice)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stringsSink = cStringArrayToSlice(cArray)
	}
}
