// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"unsafe"

	model "github.com/DataDog/agent-payload/v5/process"
)

type payloadList interface {
	Len() int
	WeightAt(idx int) int
	ToChunk(start, end int, weight int)
}

type chunkAllocator interface {
	TakenSize() int
	TakenWeight() int
	Append()
	Next()
}

func chunkPayloadsBySizeAndWeight(l payloadList, a chunkAllocator, maxChunkSize int, maxChunkWeight int) {
	start := 0
	chunkWeight := 0
	availableSize := maxChunkSize - a.TakenSize()
	availableWeight := maxChunkWeight - a.TakenWeight()
	for i := 0; i < l.Len(); i++ {
		itemWeight := l.WeightAt(i)
		size := i - start
		skipItem := false
		if size >= availableSize || chunkWeight+itemWeight > availableWeight {
			if size > 0 {
				l.ToChunk(start, i, chunkWeight)
				a.Next()
			}

			chunkWeight = 0
			start = i
			if itemWeight >= maxChunkWeight {
				if availableWeight < maxChunkWeight {
					a.Append()
				}
				l.ToChunk(i, i+1, itemWeight)
				a.Next()
				start = i + 1
				skipItem = true
			} else {
				for maxChunkSize-a.TakenSize() < 1 || maxChunkWeight-a.TakenWeight() < itemWeight {
					a.Next()
				}
			}
			availableSize = maxChunkSize - a.TakenSize()
			availableWeight = maxChunkWeight - a.TakenWeight()
		}
		if !skipItem {
			chunkWeight += itemWeight
		}
	}
	if start < l.Len() {
		l.ToChunk(start, l.Len(), chunkWeight)
	}
}

func chunkProcessesBySizeAndWeight(procs []*model.Process, maxChunkSize, maxChunkWeight int, chunker *collectorProcChunker) {
	list := &processList{
		procs:   procs,
		chunker: chunker,
	}
	chunkPayloadsBySizeAndWeight(list, chunker, maxChunkSize, maxChunkWeight)
}

type processList struct {
	procs   []*model.Process
	chunker processChunker
}

type processChunker interface {
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

type chunkProps struct {
	weight int
	size   int
}
type chunkPropsTracker struct {
	props []chunkProps
	idx   int
}

func (c *chunkPropsTracker) TakenSize() int {
	if c.idx < len(c.props) {
		return c.props[c.idx].size
	}
	return 0
}

func (c *chunkPropsTracker) TakenWeight() int {
	if c.idx < len(c.props) {
		return c.props[c.idx].weight
	}
	return 0
}
func (c *chunkPropsTracker) Append() {
	c.idx = len(c.props)
}

func (c *chunkPropsTracker) Next() {
	c.idx++
}

type collectorProcChunker struct {
	chunkPropsTracker
	container      *model.Container
	collectorProcs []*model.CollectorProc
}

func (c *collectorProcChunker) Accept(procs []*model.Process, weight int) {
	if c.idx >= len(c.collectorProcs) {
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

func weighProcess(proc *model.Process) int {
	weight := procSizeof
	if proc.Command != nil {
		weight += len(proc.Command.Exe) + len(proc.Command.Cwd) + len(proc.Command.Root)
		for _, arg := range proc.Command.Args {
			weight += len(arg)
		}
	}
	if proc.User != nil {
		weight += len(proc.User.Name)
	}
	return weight
}
