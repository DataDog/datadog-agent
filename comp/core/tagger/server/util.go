// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"iter"

	"google.golang.org/protobuf/proto"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// splitBySize splits the given slice into contiguous non-overlapping subslices such that
// the size of each sub-slice is at most maxChunkSize.
// The size of each item is calculated using computeSize
//
// This function assumes that the size of each single item of the initial slice is not larger than maxChunkSize
func splitBySize[T any](slice []T, maxChunkSize int, computeSize func(T) int) [][]T {

	// TODO: return an iter.Seq[[]T] instead of [][]T once we upgrade to golang v1.23
	// returning iter.Seq[[]T] has better performance in terms of memory consumption
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

// splitEvents splits the array of events to chunks with at most maxChunkSize each
func splitEvents(events []*pb.StreamTagsEvent, maxChunkSize int) [][]*pb.StreamTagsEvent {
	return splitBySize(
		events,
		maxChunkSize,
		func(event *pb.StreamTagsEvent) int { return proto.Size(event) },
	)
}

// splitBySize lazily splits the given slice into contiguous non-overlapping subslices such that
// the size of each sub-slice is at most maxChunkSize.
// The size of each item is calculated using computeSize
//
// It is assumed that the size of each single item of the initial slice is not larger than maxChunkSize
// This function performs lazy chunking by returning an iterator Sequence, providing a better memory performance
// by loading only one chunk into memory at a time.
func splitBySizeLazy[T any](slice []T, maxChunkSize int, computeSize func(T) int) iter.Seq[[]T] {
	return func(yield func([]T) bool) {
		chunkSize := 0
		chunk := make([]T, 0)
		for _, item := range slice {
			eventSize := computeSize(item)
			if chunkSize+eventSize > maxChunkSize {
				if !yield(chunk) {
					return
				}
				chunk = []T{}
				chunkSize = 0
			}
			chunk = append(chunk, item)
			chunkSize += eventSize
		}

		if chunkSize > 0 {
			yield(chunk)
		}
	}
}

// splitEventsLazy lazily splits the array of events to chunks with at most maxChunkSize each
// This function performs lazy chunking by returning an iterator Sequence, providing a better memory performance
// by loading only one chunk into memory at a time.
func splitEventsLazy(events []*pb.StreamTagsEvent, maxChunkSize int) iter.Seq[[]*pb.StreamTagsEvent] {
	return splitBySizeLazy(
		events,
		maxChunkSize,
		func(event *pb.StreamTagsEvent) int { return proto.Size(event) },
	)
}
