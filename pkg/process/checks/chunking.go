// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"unsafe"

	model "github.com/DataDog/agent-payload/v5/process"
)

// payloadList is an abstract list of payloads subject to chunking
type payloadList interface {
	// Len returns the length of the list
	Len() int
	// WeightAt returns weight for the payload at position `idx` in the list
	WeightAt(idx int) int
	// ToChunk copies a slice from the list to an abstract connected chunker providing the cumulative weight of the chunk
	ToChunk(start, end int, weight int)
}

// chunkAllocator abstracts management operations for chunk allocation
type chunkAllocator interface {
	// TakenSize returns the size allocated to the current chunk
	TakenSize() int
	// TakenWeight returns the weight allocated to the current chunk
	TakenWeight() int
	// Append creates a new chunk at the end (cases when it is known any previously allocated chunks cannot fit the payload)
	Append()
	// Next moves to the next chunk or allocates a new chunk if the current chunk is the last
	Next()
}

// chunkPayloadsBySizeAndWeight allocates chunks of payloads taking max allowed size and max allowed weight
// algorithm in the nutshell:
// - iterate through payloads in the `payloadList`
// - keep track of size and weight available for allocation (`TakenSize` and `TakenWeight`)
// - create a new chunk once we exceed these limits
// - consider case when the current item exceeds the max allowed weight and create a new chunk at the end (`Append`)
// this implementation allows for multiple pases through the chunks, which can be useful in cases with different payload types
// being allocated within chunks
func chunkPayloadsBySizeAndWeight(l payloadList, a chunkAllocator, maxChunkSize int, maxChunkWeight int) {
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

// chunkProcessesBySizeAndWeight chunks `model.Process` payloads by max allowed size and max allowed weight of a chunk
func chunkProcessesBySizeAndWeight(procs []*model.Process, ctr *model.Container, maxChunkSize, maxChunkWeight int, chunker *collectorProcChunker) {
	// Use the last available chunk as it may have some space for payloads
	chunker.idx = 0
	if len(chunker.collectorProcs) > 1 {
		chunker.idx = len(chunker.collectorProcs) - 1
	}
	// Processs that have a related container will add this container to every chunk they are split across
	// This may result in the same container being sent in multiple payloads from the agent
	// Note that this is necessary because container process tags (sent within `model.Container`) are only resolved from
	// containers seen within the same `model.ContainerProc` as processes.
	chunker.container = ctr
	list := &processList{
		procs:   procs,
		chunker: chunker,
	}
	chunkPayloadsBySizeAndWeight(list, chunker, maxChunkSize, maxChunkWeight)
}

// processList is a payload list of `model.Process` payloads
type processList struct {
	procs   []*model.Process
	chunker processChunker
}

// processChunker abstracts chunking of `model.Process` payloads
type processChunker interface {
	// Accept takes a slice of `model.Process` and allocates them to the current chunk
	Accept(procs []*model.Process, weight int)
}

func (l *processList) Len() int {
	return len(l.procs)
}

func (l *processList) WeightAt(idx int) int {
	if idx >= len(l.procs) {
		return 0
	}
	return weighProcess(l.procs[idx])
}

func (l *processList) ToChunk(start, end int, weight int) {
	l.chunker.Accept(l.procs[start:end], weight)
}

// chunkProps is used to track weight and size of chunks
type chunkProps struct {
	weight int
	size   int
}

// chunkPropsTracker tracks weight and size of chunked payloads
type chunkPropsTracker struct {
	props []chunkProps
	idx   int
}

// TakenSize returns the size allocated to the current chunk
func (c *chunkPropsTracker) TakenSize() int {
	if c.idx < len(c.props) {
		return c.props[c.idx].size
	}
	return 0
}

// TakenWeight returns the weight allocated to the current chunk
func (c *chunkPropsTracker) TakenWeight() int {
	if c.idx < len(c.props) {
		return c.props[c.idx].weight
	}
	return 0
}

// Append creates a new chunk at the end (cases when it is known any previously allocated chunks cannot fit the payload)
func (c *chunkPropsTracker) Append() {
	c.idx = len(c.props)
}

// Next moves to the next chunk or allocates a new chunk if the current chunk is the last
func (c *chunkPropsTracker) Next() {
	c.idx++
}

// collectorProcChunker implements allocation of chunks to `model.CollectorProc`
type collectorProcChunker struct {
	chunkPropsTracker
	container      *model.Container
	collectorProcs []*model.CollectorProc
}

// collectprProcChunker implements both `chunkAllocator` and `processChunker`
var _ processChunker = &collectorProcChunker{}
var _ chunkAllocator = &collectorProcChunker{}

func (c *collectorProcChunker) Accept(procs []*model.Process, weight int) {
	if c.idx >= len(c.collectorProcs) {
		// If we are outside of the range of allocated chunks, allocate a new one
		c.collectorProcs = append(c.collectorProcs, &model.CollectorProc{})
		c.props = append(c.props, chunkProps{})
	}

	collectorProc := c.collectorProcs[c.idx]

	// Note that we are currently not accounting for the container size/weight in calculations
	if c.container != nil {
		collectorProc.Containers = append(collectorProc.Containers, c.container)
	}
	collectorProc.Processes = append(collectorProc.Processes, procs...)
	c.props[c.idx].size += len(procs)
	c.props[c.idx].weight += weight
}

var procSizeof = int(unsafe.Sizeof(model.Process{}))
var procSizeofSerializeDelta = (&model.Process{}).Size() -
func weighProcess(proc *model.Process) int {
	weight := procSizeof
	if proc.Command != nil {
		weight += len(proc.Command.Exe) + len(proc.Command.Cwd) + len(proc.Command.Root)
		for _, arg := range proc.Command.Args {
			weight += 2*len(arg)
		}
	}
	if proc.User != nil {
		weight += 2*len(proc.User.Name)
	}
	return weight
}
