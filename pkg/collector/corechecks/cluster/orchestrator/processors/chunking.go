// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator
// +build orchestrator

package processors

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// orchestratorMetadataList is a payload list of orchestratorMetadata payloads
type orchestratorMetadataList struct {
	orchestratorMetadataYaml [][]byte
	orchestratorMetadata     []interface{}
	chunker                  orchestratorMetadataChunker
}

// orchestratorMetadataChunker abstracts chunking of orchestratorMetadata payloads
type orchestratorMetadataChunker interface {
	// Accept takes a slice of orchestratorMetadata and allocates them to the current chunk
	Accept(orchestratorMetadata []interface{}, weight int)
}

func (l *orchestratorMetadataList) Len() int {
	return len(l.orchestratorMetadata)
}

func (l *orchestratorMetadataList) WeightAt(idx int) int {
	if idx >= len(l.orchestratorMetadata) {
		return 0
	}
	return len(l.orchestratorMetadataYaml[idx])
}

func (l *orchestratorMetadataList) ToChunk(start, end int, weight int) {
	l.chunker.Accept(l.orchestratorMetadata[start:end], weight)
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
type collectorOrchestratorMetadataChunker struct {
	chunkPropsTracker
	collectorOrchestratorMetadataList [][]interface{}
}

// collectorOrchestratorMetadataChunker implements both `chunkAllocator` and `orchestratorMetadataChunker`
var _ orchestratorMetadataChunker = &collectorOrchestratorMetadataChunker{}
var _ util.ChunkAllocator = &collectorOrchestratorMetadataChunker{}

func (c *collectorOrchestratorMetadataChunker) Accept(orchestratorMetadata []interface{}, weight int) {
	if c.idx >= len(c.collectorOrchestratorMetadataList) {
		// If we are outside of the range of allocated chunks, allocate a new one
		c.collectorOrchestratorMetadataList = append(c.collectorOrchestratorMetadataList, make([]interface{}, 0, 1))
		c.props = append(c.props, chunkProps{})
	}
	c.collectorOrchestratorMetadataList[c.idx] = append(c.collectorOrchestratorMetadataList[c.idx], orchestratorMetadata...)
	c.props[c.idx].size += len(orchestratorMetadata)
	c.props[c.idx].weight += weight
}
func (c *collectorOrchestratorMetadataChunker) setLastChunk() {
	c.idx = 0
	if len(c.collectorOrchestratorMetadataList) > 1 {
		c.idx = len(c.collectorOrchestratorMetadataList) - 1
	}
}

// chunkOrchestratorMetadataBySizeAndWeight chunks orchestratorMetadata payloads by max allowed size and max allowed weight of a chunk
func chunkOrchestratorMetadataBySizeAndWeight(orchestratorMetadata []interface{}, orchestratorMetadataYaml [][]byte, maxChunkSize, maxChunkWeight int, chunker *collectorOrchestratorMetadataChunker) {
	if len(orchestratorMetadata) == 0 {
		return
	}

	// Use the last available chunk as it may have some space for payloads
	chunker.setLastChunk()

	list := &orchestratorMetadataList{
		orchestratorMetadata:     orchestratorMetadata,
		orchestratorMetadataYaml: orchestratorMetadataYaml,
		chunker:                  chunker,
	}
	util.ChunkPayloadsBySizeAndWeight(list, chunker, maxChunkSize, maxChunkWeight)

	return
}
