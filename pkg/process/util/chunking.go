// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

// PayloadList is an abstract list of payloads subject to chunking
/*
type PayloadList interface {
	// Len returns the length of the list
	Len() int
	// WeightAt returns weight for the payload at position `idx` in the list
	WeightAt(idx int) int
	// ToChunk copies a slice from the list to an abstract connected chunker providing the cumulative weight of the chunk
	ToChunk(start, end int, weight int)
}
*/

type WeightAt func(int) int

type PayloadList[T any] struct {
	Items    []*T
	WeightAt WeightAt
}

func (l *PayloadList[T]) Len() int {
	return len(l.Items)
}

// chunkProps is used to track weight and size of chunks
type chunkProps struct {
	weight int
	size   int
}

type AppendToChunk[T any, P any] func(t *T, ps []*P)
type NewChunk[T any] func() *T
type OnAccept[T any] func(t *T)

// ChunkAllocator abstracts management operations for chunk allocation
type ChunkAllocator[T any, P any] struct {
	props  []chunkProps
	idx    int
	chunks []*T

	NewChunk      NewChunk[T]
	AppendToChunk AppendToChunk[T, P]
	OnAccept      OnAccept[T]
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

func (c *ChunkAllocator[T, P]) SetLastChunk() {
	c.idx = 0
	if len(c.chunks) > 1 {
		c.idx = len(c.chunks) - 1
	}
}

func (c *ChunkAllocator[T, P]) Accept(ps []*P, weight int) {
	if c.idx >= len(c.chunks) {
		// If we are outside of the range of allocated chunks, allocate a new one
		c.chunks = append(c.chunks, c.NewChunk())
		c.props = append(c.props, chunkProps{})
	}

	c.AppendToChunk(c.chunks[c.idx], ps)
	c.props[c.idx].size += len(ps)
	c.props[c.idx].weight += weight
	c.OnAccept(c.chunks[c.idx])
}

func (c *ChunkAllocator[T, P]) GetChunks() *[]*T {
	return &c.chunks
}

/*
// TakenSize returns the size allocated to the current chunk
TakenSize() int
// TakenWeight returns the weight allocated to the current chunk
TakenWeight() int
// Append creates a new chunk at the end (cases when it is known any previously allocated chunks cannot fit the payload)
Append()
// Next moves to the next chunk or allocates a new chunk if the current chunk is the last
Next()
*/

// ChunkPayloadsBySizeAndWeight allocates chunks of payloads taking max allowed size and max allowed weight
// algorithm in the nutshell:
// - iterate through payloads in the `payloadList`
// - keep track of size and weight available for allocation (`TakenSize` and `TakenWeight`)
// - create a new chunk once we exceed these limits
// - consider case when the current item exceeds the max allowed weight and create a new chunk at the end (`Append`)
// this implementation allows for multiple passes through the chunks, which can be useful in cases with different payload types
// being allocated within chunks
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
				a.Accept(l.Items[i:i+1], chunkWeight)
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

/*
func ChunkPayloadsBySizeAndWeight2(l[], w []int, maxChunkSize int, maxChunkWeight int) {
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
*/
