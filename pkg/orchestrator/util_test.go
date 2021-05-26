/*
 * Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-2021 Datadog, Inc.
 */

package orchestrator

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

// TestChunkRange tests the chunking function. Note that the chunkCount is actually not given but calculated by running:
// 	chunkCount := elements / chunkSize
//	if elements%chunkSize != 0 {
//		chunkCount++
//	}
func TestChunkRange(t *testing.T) {
	type want struct {
		start int
		end   int
	}
	tests := []struct {
		name       string
		chunkCount int
		chunkSize  int
		elements   int
		want       []want
	}{
		{
			name:       "no chunks",
			chunkCount: 0,
			chunkSize:  0,
			elements:   10,
			want:       []want{{}},
		},
		{
			name:       "3 chunks, size 1, 10 elements",
			chunkCount: 3,
			chunkSize:  1,
			elements:   10,
			want:       []want{{0, 1}, {1, 2}, {2, 10}},
		}, {

			name:       "2 chunks, size 2, 5 elements",
			chunkCount: 2,
			chunkSize:  2,
			elements:   5,
			want:       []want{{0, 2}, {2, 5}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for counter := 1; counter <= tt.chunkCount; counter++ {
				chunkStart, chunkEnd := ChunkRange(tt.elements, tt.chunkCount, tt.chunkSize, counter)
				assert.Equal(t, tt.want[counter-1].start, chunkStart)
				assert.Equal(t, tt.want[counter-1].end, chunkEnd)
			}
		})
	}
}
