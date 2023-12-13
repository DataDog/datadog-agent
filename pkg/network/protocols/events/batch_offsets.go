// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package events

import (
	"sync"
)

// offsetManager is responsible for keeping track of which chunks of data we
// have consumed from each batch object
type offsetManager struct {
	mux        sync.Mutex
	stateByCPU []*cpuReadState
}

type cpuReadState struct {
	// this is the nextBatchID we're expecting for a particular CPU core. we use
	// this when we attempt to retrieve data that hasn't been sent from kernel space
	// yet because it belongs to an incomplete batch.
	nextBatchID int

	// information associated to partial batch reads
	partialBatchID int
	partialOffset  int
}

func newOffsetManager(numCPUS int) *offsetManager {
	stateByCPU := make([]*cpuReadState, numCPUS)
	for i := range stateByCPU {
		stateByCPU[i] = new(cpuReadState)
	}

	return &offsetManager{stateByCPU: stateByCPU}
}

// Get returns the data offset that hasn't been consumed yet for a given batch
func (o *offsetManager) Get(cpu int, batch *batch, syncing bool) (begin, end int) {
	o.mux.Lock()
	defer o.mux.Unlock()
	state := o.stateByCPU[cpu]
	batchID := int(batch.Idx)

	if batchID < state.nextBatchID {
		// we have already consumed this data
		return 0, 0
	}

	if batchComplete(batch) {
		state.nextBatchID = batchID + 1
	}

	// determining the begin offset
	// usually this is 0, but if we've done a partial read of this batch
	// we need to take that into account
	if int(batch.Idx) == state.partialBatchID {
		begin = state.partialOffset
	}

	// determining the end offset
	// usually this is the full batch size but it can be less
	// in the context of a forced (partial) read
	end = int(batch.Len)

	// if this is part of a forced read (that is, we're reading a batch before
	// it's complete) we need to keep track of which entries we're reading
	// so we avoid reading the same entries again
	if syncing {
		state.partialBatchID = int(batch.Idx)
		state.partialOffset = end
	}

	return
}

func (o *offsetManager) NextBatchID(cpu int) int {
	o.mux.Lock()
	defer o.mux.Unlock()

	return o.stateByCPU[cpu].nextBatchID
}

func max(a, b int) int {
	if a >= b {
		return a
	}

	return b
}

func batchComplete(b *batch) bool {
	return b.Cap > 0 && b.Len == b.Cap
}
