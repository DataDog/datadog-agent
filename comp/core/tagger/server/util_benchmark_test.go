// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package server

import (
	"testing"
)

const baseSliceLength = 10000000

var baseBenchmarkSlice = make([]int, baseSliceLength, baseSliceLength)

func init() {
	for i := range baseSliceLength {
		baseBenchmarkSlice[i] = i
	}
}

func BenchmarkChunkLazyChunking(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for chunk := range splitBySizeLazy(baseBenchmarkSlice, 1, func(x int) int { return x }) {
			_ = chunk // Use chunk to avoid compiler optimizations
		}
	}
}

func BenchmarkChunkSliceChunking(b *testing.B) {
	for i := 0; i < b.N; i++ {
		chunks := splitBySize(baseBenchmarkSlice, 1, func(x int) int { return x })
		for _, chunk := range chunks {
			_ = chunk // Use chunks to avoid compiler optimizations
		}
	}
}
