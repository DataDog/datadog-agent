// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package events contains implementation to unify perf-map communication between kernel and user space.
package events

import (
	"fmt"
	"sync"
)

// offsetManager is responsible for keeping track of which chunks of data we
// have consumed from each batch object
type offsetManager struct {
	mux        sync.Mutex
	stateByCPU []*cpuReadState
	debug      bool
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
func (o *offsetManager) Get(cpu int, batch *Batch, syncing bool) (begin, end int) {
	o.mux.Lock()
	defer o.mux.Unlock()
	state := o.stateByCPU[cpu]
	if o.debug {
		fmt.Printf("[batch-offsets] Get state for cpu %d; state: %#v\n", cpu, state)
	}
	batchID := int(batch.Idx)

	if batchID < state.nextBatchID {
		// we have already consumed this data
		return 0, 0
	}

	fmt.Printf("[batch-offsets] Get begin for batch idx: %d; cpu: %d; len: %d; cap: %d; event_size: %d; dropped events: %d; failed_flushes: %d\n", batch.Idx, batch.Cpu, batch.Len, batch.Cap, batch.Event_size, batch.Dropped_events, batch.Failed_flushes)
	if batchComplete(batch) {
		state.nextBatchID = batchID + 1
	}

	// determining the begin offset
	// usually this is 0, but if we've done a partial read of this batch
	// we need to take that into account
	if int(batch.Idx) == state.partialBatchID {
		if o.debug {
			fmt.Printf("[batch-offsets] using partial begin data for cpu %d; begin: %#v\n", cpu, state.partialOffset)
		}
		begin = state.partialOffset
	}

	// determining the end offset
	// usually this is the full batch size but it can be less
	// in the context of a forced (partial) read
	end = int(batch.Len)
	if end == 0 {
		return begin, begin
	}

	// if this is part of a forced read (that is, we're reading a batch before
	// it's complete) we need to keep track of which entries we're reading
	// so we avoid reading the same entries again
	if syncing {
		if o.debug {
			fmt.Printf("[batch-offsets] overriding syncing data for cpu %d; state before: %#v\n", cpu, state)
		}
		state.partialBatchID = int(batch.Idx)
		state.partialOffset = end
		if o.debug {
			fmt.Printf("[batch-offsets] overriding syncing data for cpu %d; state after: %#v\n", cpu, state)
		}
	}

	return
}

func (o *offsetManager) NextBatchID(cpu int) int {
	o.mux.Lock()
	defer o.mux.Unlock()

	return o.stateByCPU[cpu].nextBatchID
}

func batchComplete(b *Batch) bool {
	return b.Cap > 0 && b.Len == b.Cap
}
