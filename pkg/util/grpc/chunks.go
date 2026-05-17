// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

// CountChunks returns the number of chunks that ProcessChunksInPlace would
// produce for the given slice and size limit.
func CountChunks[T any](slice []T, maxChunkSize int, computeSize func(T) int) int {
	count := 0
	idx := 0
	for idx < len(slice) {
		chunkSize := computeSize(slice[idx])
		j := idx + 1
		for j < len(slice) {
			s := computeSize(slice[j])
			if chunkSize+s > maxChunkSize {
				break
			}
			chunkSize += s
			j++
		}
		count++
		idx = j
	}
	return count
}

// ProcessChunksInPlace splits the slice into contiguous chunks whose total size
// is at most maxChunkSize and calls consume on each chunk. If a single item
// exceeds maxChunkSize it is sent alone in a singleton chunk.
//
// The size of each item is computed with computeSize. No extra memory is
// allocated — consume receives sub-slices of the original slice.
func ProcessChunksInPlace[T any](slice []T, maxChunkSize int, computeSize func(T) int, consume func([]T) error) error {
	idx := 0
	for idx < len(slice) {
		chunkSize := computeSize(slice[idx])
		j := idx + 1

		for j < len(slice) {
			eventSize := computeSize(slice[j])
			if chunkSize+eventSize > maxChunkSize {
				break
			}
			chunkSize += eventSize
			j++
		}

		if err := consume(slice[idx:j]); err != nil {
			return err
		}
		idx = j
	}
	return nil
}
