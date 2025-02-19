// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package server

import (
	"fmt"
	"testing"
)

var benchmarkSizes = []int{100, 1000, 10000, 100000, 1000000}

const maxChunkSize = 4
const mockItemSize = 1

var global []int

func createBaseBenchmarkSlice(size int) []int {
	var baseBenchmarkSlice = make([]int, size)
	for i := range size {
		baseBenchmarkSlice[i] = i
	}
	return baseBenchmarkSlice
}

func mockComputeSize(int) int { return mockItemSize }

func BenchmarkProcessChunks(b *testing.B) {
	b.ReportAllocs()
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("%d-items", size), func(b *testing.B) {

			// Point this to the implementation you want to benchmark
			var processChunksFunc = processChunksInPlace[int]

			items := createBaseBenchmarkSlice(size)
			var local []int

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = processChunksFunc(items, maxChunkSize, mockComputeSize, func(t []int) error { local = t; return nil })
			}
			global = local
		})
	}
}
