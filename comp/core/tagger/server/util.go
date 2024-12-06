// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
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
