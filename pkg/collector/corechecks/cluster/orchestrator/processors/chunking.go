// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator
// +build orchestrator

package processors

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// orchestratorList is a payload list of orchestrator payloads
type orchestratorList struct {
	// We use yaml size as weight indicator
	orchestratorYaml []interface{}
	// Slice of payloads to be chunked, it's a generic attribute for metadata payloads and manifest payloads
	orchestratorPayloads []interface{}
	chunker              orchestratorChunker
}

// orchestratorChunker abstracts chunking of orchestrator payloads
type orchestratorChunker interface {
	// Accept takes a slice of orchestrator payloads and allocates them to the current chunk
	Accept(orchestrator []interface{}, weight int)
}

func (l *orchestratorList) Len() int {
	return len(l.orchestratorPayloads)
}

func (l *orchestratorList) WeightAt(idx int) int {
	if idx >= len(l.orchestratorPayloads) {
		return 0
	}
	return len(l.orchestratorYaml[idx].(*model.Manifest).Content)
}

func (l *orchestratorList) ToChunk(start, end int, weight int) {
	l.chunker.Accept(l.orchestratorPayloads[start:end], weight)
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

// collectorProcChunker implements allocation of chunks
type collectorOrchestratorChunker struct {
	chunkPropsTracker
	collectorOrchestratorList [][]interface{}
}

// collectorOrchestratorChunker implements both `chunkAllocator` and `orchestratorChunker`
var _ orchestratorChunker = &collectorOrchestratorChunker{}
var _ util.ChunkAllocator = &collectorOrchestratorChunker{}

func (c *collectorOrchestratorChunker) Accept(orchestratorPayloads []interface{}, weight int) {
	if c.idx >= len(c.collectorOrchestratorList) {
		// If we are outside of the range of allocated chunks, allocate a new one
		c.collectorOrchestratorList = append(c.collectorOrchestratorList, make([]interface{}, 0, 1))
		c.props = append(c.props, chunkProps{})
	}
	c.collectorOrchestratorList[c.idx] = append(c.collectorOrchestratorList[c.idx], orchestratorPayloads...)
	c.props[c.idx].size += len(orchestratorPayloads)
	c.props[c.idx].weight += weight
}
func (c *collectorOrchestratorChunker) setLastChunk() {
	c.idx = 0
	if len(c.collectorOrchestratorList) > 1 {
		c.idx = len(c.collectorOrchestratorList) - 1
	}
}

// chunkOrchestratorPayloadsBySizeAndWeight chunks orchestrator payloads by max allowed size and max allowed weight of a chunk
// We use yaml size as payload weight
func chunkOrchestratorPayloadsBySizeAndWeight(orchestratorPayloads []interface{}, orchestratorYaml []interface{}, maxChunkSize, maxChunkWeight int, chunker *collectorOrchestratorChunker) {
	if len(orchestratorPayloads) == 0 {
		return
	}

	// Use the last available chunk as it may have some space for payloads
	chunker.setLastChunk()

	list := &orchestratorList{
		orchestratorPayloads: orchestratorPayloads,
		orchestratorYaml:     orchestratorYaml,
		chunker:              chunker,
	}
	util.ChunkPayloadsBySizeAndWeight(list, chunker, maxChunkSize, maxChunkWeight)

	return
}
