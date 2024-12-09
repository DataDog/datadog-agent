// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"google.golang.org/protobuf/proto"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type sizeComputerFunc[T any] func(T) int

type consumeChunkFunc[T any] func(T) error

// computeProtoEventSize returns the size of a tags stream event in bytes
func computeTagsEventInBytes(event *pb.StreamTagsEvent) int { return proto.Size(event) }

// processChunksInPlace splits the passed slice into contiguous chunks such that the total size of each chunk is at most maxChunkSize
// and applies the consume function to each of these chunks
//
// The size of an item is computed with computeSize
// If an item has a size large than maxChunkSize, it is placed in a singleton chunk (chunk with one item)
//
// The consume function is applied to different chunks in-place, without any need extra memory allocation
func processChunksInPlace[T any](slice []T, maxChunkSize int, computeSize sizeComputerFunc[T], consume consumeChunkFunc[[]T]) error {
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

func splitBySize[T any](slice []T, maxChunkSize int, computeSize func(T) int) [][]T {
	var chunks [][]T
	currentChunk := []T{}
	currentSize := 0

	for _, item := range slice {
		eventSize := computeSize(item)
		if currentSize+eventSize > maxChunkSize {
			chunks = append(chunks, currentChunk)
			currentChunk = []T{}
			currentSize = 0
		}
		currentChunk = append(currentChunk, item)
		currentSize += eventSize
	}
	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}
	return chunks
}

// processChunksWithSplit splits the passed slice into contiguous chunks such that the total size of each chunk is at most maxChunkSize
// and then applies the consume function to each of these chunks
//
// The size of an item is computed with computeSize
// If an item has a size large than maxChunkSize, it is placed in a singleton chunk (chunk with one item)
//
// Prefer using processChunksInPlace for better CPU and memory performance. This implementation is only kept for benchmarking purposes.
func processChunksWithSplit[T any](slice []T, maxChunkSize int, computeSize sizeComputerFunc[T], consume consumeChunkFunc[[]T]) error {
	chunks := splitBySize(slice, maxChunkSize, computeSize)

	for _, chunk := range chunks {
		if err := consume(chunk); err != nil {
			return err
		}
	}

	return nil
}
