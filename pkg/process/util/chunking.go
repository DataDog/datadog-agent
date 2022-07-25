// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

// PayloadList is an abstract list of payloads subject to chunking
type PayloadList interface {
	// Len returns the length of the list
	Len() int
	// WeightAt returns weight for the payload at position `idx` in the list
	WeightAt(idx int) int
	// ToChunk copies a slice from the list to an abstract connected chunker providing the cumulative weight of the chunk
	ToChunk(start, end int, weight int)
}

// ChunkAllocator abstracts management operations for chunk allocation
type ChunkAllocator interface {
	// TakenSize returns the size allocated to the current chunk
	TakenSize() int
	// TakenWeight returns the weight allocated to the current chunk
	TakenWeight() int
	// Append creates a new chunk at the end (cases when it is known any previously allocated chunks cannot fit the payload)
	Append()
	// Next moves to the next chunk or allocates a new chunk if the current chunk is the last
	Next()
}

// ChunkPayloadsBySizeAndWeight allocates chunks of payloads taking max allowed size and max allowed weight
// algorithm in the nutshell:
// - iterate through payloads in the `payloadList`
// - keep track of size and weight available for allocation (`TakenSize` and `TakenWeight`)
// - create a new chunk once we exceed these limits
// - consider case when the current item exceeds the max allowed weight and create a new chunk at the end (`Append`)
// this implementation allows for multiple passes through the chunks, which can be useful in cases with different payload types
// being allocated within chunks
func ChunkPayloadsBySizeAndWeight(l PayloadList, a ChunkAllocator, maxChunkSize int, maxChunkWeight int) {
	start := 0
	chunkWeight := 0
	// Take available size and available weight by consulting the current chunk
	availableSize := maxChunkSize - a.TakenSize()
	availableWeight := maxChunkWeight - a.TakenWeight()
	for i := 0; i < l.Len(); i++ {
		itemWeight := l.WeightAt(i)
		// Evaluate size of the currently accumulated items (from the start of the candidate chunk)
		size := i - start
		// Track if we need to skeep the item on the next chunk (large item chunked separately)
		skipItem := false
		if size >= availableSize || chunkWeight+itemWeight > availableWeight {
			// We are exceeding available size or available weight and it is time to create a new chunk
			if size > 0 {
				// We already accumulated some items - create a new chunk
				l.ToChunk(start, i, chunkWeight)
				a.Next()
			}
			// Reset chunk weight
			chunkWeight = 0
			// Reset chunk start position
			start = i
			// Check if the current item exceeds the max allowed chunk weight
			if itemWeight >= maxChunkWeight {
				// Current item is exceeding max allowed chunk weight and should be chunked separately
				if availableWeight < maxChunkWeight {
					// Currently considered chunk already has allocations - create a new chunk at the end
					a.Append()
				}
				// Chunk a single iem
				l.ToChunk(i, i+1, itemWeight)
				a.Next()
				// Skip over this single item
				start = i + 1
				skipItem = true
			} else {
				// Find a chunk that can take the current items
				for maxChunkSize-a.TakenSize() < 1 || maxChunkWeight-a.TakenWeight() < itemWeight {
					a.Next()
				}
			}
			// Reset available size and available weight based ont he current chunk
			availableSize = maxChunkSize - a.TakenSize()
			availableWeight = maxChunkWeight - a.TakenWeight()
		}
		if !skipItem {
			// Only include the current item if it hasn't been to a separate chunk
			chunkWeight += itemWeight
		}
	}
	// Chunk the remainder of payloads
	if start < l.Len() {
		l.ToChunk(start, l.Len(), chunkWeight)
	}
}
