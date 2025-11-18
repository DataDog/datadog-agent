// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

//nolint:revive // TODO(PROC) Fix revive linter
type WeightAt func(int) int

// PayloadList is a wrapper for payloads subject to chunking
type PayloadList[T any] struct {
	// The items to chunk
	Items []T
	// A function which returns the weight of an item at the given index
	WeightAt WeightAt
}

// Len returns the number of items in the list
func (l *PayloadList[T]) Len() int {
	return len(l.Items)
}

// chunkProps is used to track weight and size of chunks
type chunkProps struct {
	weight int
	size   int
}

//nolint:revive // TODO(PROC) Fix revive linter
type AppendToChunk[T any, P any] func(t *T, ps []P)

//nolint:revive // TODO(PROC) Fix revive linter
type OnAccept[T any] func(t *T)

// ChunkAllocator manages operations for chunk allocation. The type T is the type of the chunk, and the type P is the
// type of the payload.
type ChunkAllocator[T any, P any] struct {
	props  []chunkProps
	idx    int
	chunks []T

	// A function which adds the group of payloads to the chunk
	AppendToChunk AppendToChunk[T, P]
	// An optional callback that allows for manipulation the chunk when a payload is added
	OnAccept OnAccept[T]
}

// TakenSize returns the size allocated to the current chunk
func (c *ChunkAllocator[T, P]) TakenSize() int {
	if c.idx < len(c.props) {
		return c.props[c.idx].size
	}
	return 0
}

// TakenWeight returns the weight allocated to the current chunk
func (c *ChunkAllocator[T, P]) TakenWeight() int {
	if c.idx < len(c.props) {
		return c.props[c.idx].weight
	}
	return 0
}

// Append creates a new chunk at the end (cases when it is known any previously allocated chunks cannot fit the payload)
func (c *ChunkAllocator[T, P]) Append() {
	c.idx = len(c.props)
}

// Next moves to the next chunk or allocates a new chunk if the current chunk is the last
func (c *ChunkAllocator[T, P]) Next() {
	c.idx++
}

// SetLastChunk sets the last chunk in case there is space at end across multiple runs
func (c *ChunkAllocator[T, P]) SetLastChunk() {
	c.idx = 0
	if len(c.chunks) > 1 {
		c.idx = len(c.chunks) - 1
	}
}

// SetActiveChunk allows for rewinding in the case of multiple runs
func (c *ChunkAllocator[T, P]) SetActiveChunk(i int) {
	c.idx = i
}

// Accept accepts a group of payloads into the current chunk
func (c *ChunkAllocator[T, P]) Accept(ps []P, weight int) {
	if c.idx >= len(c.chunks) {
		// If we are outside of the range of allocated chunks, allocate a new one
		c.chunks = append(c.chunks, *new(T))
		c.props = append(c.props, chunkProps{})
	}

	if c.OnAccept != nil {
		c.OnAccept(&c.chunks[c.idx])
	}
	c.AppendToChunk(&c.chunks[c.idx], ps)
	c.props[c.idx].size += len(ps)
	c.props[c.idx].weight += weight
}

//nolint:revive // TODO(PROC) Fix revive linter
func (c *ChunkAllocator[T, P]) GetChunks() *[]T {
	return &c.chunks
}

// ChunkPayloadsBySizeAndWeight allocates chunks of payloads taking max allowed size and max allowed weight
// algorithm in the nutshell:
// - iterate through payloads in the `payloadList`
// - keep track of size and weight available for allocation (`TakenSize` and `TakenWeight`)
// - create a new chunk once we exceed these limits
// - consider case when the current item exceeds the max allowed weight and create a new chunk at the end (`Append`)
// this implementation allows for multiple passes through the chunks, which can be useful in cases with different payload types
// being allocated within chunks
// See PayloadList and ChunkAllocator for a description of the type params.
func ChunkPayloadsBySizeAndWeight[T any, P any](l *PayloadList[P], a *ChunkAllocator[T, P], maxChunkSize int, maxChunkWeight int) {
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
				a.Accept(l.Items[start:i], chunkWeight)
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
				a.Accept(l.Items[i:i+1], itemWeight)
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
			// Reset available size and available weight based on the current chunk
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
		a.Accept(l.Items[start:], chunkWeight)
	}
}
