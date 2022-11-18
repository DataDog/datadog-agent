// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// chunkProcessesBySizeAndWeight chunks `model.Process` payloads by max allowed size and max allowed weight of a chunk
func chunkProcessesBySizeAndWeight(procs []*model.Process, ctr *model.Container, maxChunkSize, maxChunkWeight int, chunker *collectorProcChunker) {
	if ctr != nil && len(procs) == 0 {
		// can happen in two scenarios, and we still need to report the container
		// a) if a process is skipped (e.g. disallowlisted)
		// b) if process <=> container mapping cannot be established (e.g. Docker on Windows)
		chunker.appendContainerWithoutProcesses(ctr)
		return
	}

	// Use the last available chunk as it may have some space for payloads
	chunker.setLastChunk()

	// Processes that have a related container will add this container to every chunk they are split across
	// This may result in the same container being sent in multiple payloads from the agent
	// Note that this is necessary because container process tags (sent within `model.Container`) are only resolved from
	// containers seen within the same `model.ContainerProc` as processes.
	chunker.container = ctr
	list := &processList{
		procs:   procs,
		chunker: chunker,
	}
	util.ChunkPayloadsBySizeAndWeight(list, chunker, maxChunkSize, maxChunkWeight)
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
var _ util.ChunkAllocator = &collectorProcChunker{}

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

func (c *collectorProcChunker) setLastChunk() {
	c.idx = 0
	if len(c.collectorProcs) > 1 {
		c.idx = len(c.collectorProcs) - 1
	}
}

func (c *collectorProcChunker) appendContainerWithoutProcesses(ctr *model.Container) {
	if len(c.collectorProcs) == 0 {
		c.collectorProcs = append(c.collectorProcs, &model.CollectorProc{})
	}
	collectorProc := c.collectorProcs[len(c.collectorProcs)-1]
	collectorProc.Containers = append(collectorProc.Containers, ctr)
}

var (
	// procSizeofSampleProcess is a sample process used in sizeof/weight calculations
	procSizeofSampleProcess = &model.Process{
		Memory:   &model.MemoryStat{},
		Cpu:      &model.CPUStat{},
		IoStat:   &model.IOStat{},
		Networks: &model.ProcessNetworks{},
	}
	// procSizeofProto is a size of the empty process
	procSizeofProto = procSizeofSampleProcess.Size()
)

// weighProcess weighs `model.Process` payloads using an approximation of a serialized size of the proto message
func weighProcess(proc *model.Process) int {
	weight := procSizeofProto
	if proc.Command != nil {
		weight += len(proc.Command.Exe) + len(proc.Command.Cwd) + len(proc.Command.Root)
		for _, arg := range proc.Command.Args {
			weight += len(arg)
		}
	}
	if proc.User != nil {
		weight += len(proc.User.Name)
	}
	weight += len(proc.ContainerId)
	return weight
}
