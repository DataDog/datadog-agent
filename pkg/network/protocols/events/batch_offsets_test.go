// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOffsets(t *testing.T) {
	const numCPUs = 2
	offsets := newOffsetManager(numCPUs)

	assert.Equal(t, 0, offsets.NextBatchID(0))
	assert.Equal(t, 0, offsets.NextBatchID(1))

	// reading full batch: cpu=0 batchID=0
	begin, end := offsets.Get(0, &batch{Idx: 0, Len: 10, Cap: 10}, false)
	assert.Equal(t, 0, begin)
	assert.Equal(t, 10, end)
	// nextBatchID is advanced to 1 for cpu=0
	assert.Equal(t, 1, offsets.NextBatchID(0))

	// reading partial batch: cpu=1 batchID=0 sync=true
	begin, end = offsets.Get(1, &batch{Idx: 0, Len: 8, Cap: 10}, true)
	assert.Equal(t, 0, begin)
	assert.Equal(t, 8, end)
	// nextBatchID remains 0 for cpu=1 since this batch hasn't been filled up yet
	assert.Equal(t, 0, offsets.NextBatchID(1))

	// reading full batch: cpu=1 batchID=0
	begin, end = offsets.Get(1, &batch{Idx: 0, Len: 10, Cap: 10}, false)
	// notice we only read now the remaining offsets
	assert.Equal(t, 8, begin)
	assert.Equal(t, 10, end)
	// nextBatchID is advanced to 1 for cpu=1
	assert.Equal(t, 1, offsets.NextBatchID(1))

	// reading partial batch: cpu=0 batchID=1 sync=true
	begin, end = offsets.Get(0, &batch{Idx: 1, Len: 4, Cap: 10}, true)
	assert.Equal(t, 0, begin)
	assert.Equal(t, 4, end)
	// nextBatchID remains 1 for cpu=0
	assert.Equal(t, 1, offsets.NextBatchID(0))

	// reading partial batch: cpu=0 batchID=1 sync=true
	begin, end = offsets.Get(0, &batch{Idx: 1, Len: 5, Cap: 10}, true)
	assert.Equal(t, 4, begin)
	assert.Equal(t, 5, end)
	// nextBatchID remains 1 for cpu=0
	assert.Equal(t, 1, offsets.NextBatchID(0))
}

func TestDelayedBatchReads(t *testing.T) {
	const numCPUs = 1
	offsets := newOffsetManager(numCPUs)

	// this emulates the scenario where we preemptively read (sync=true) two
	// complete batches in a row before they are read from perf buffer
	begin, end := offsets.Get(0, &batch{Idx: 0, Len: 10, Cap: 10}, true)
	assert.Equal(t, 0, begin)
	assert.Equal(t, 10, end)

	begin, end = offsets.Get(0, &batch{Idx: 1, Len: 10, Cap: 10}, true)
	assert.Equal(t, 0, begin)
	assert.Equal(t, 10, end)

	// now the "delayed" batches from perf buffer are read in sequence
	begin, end = offsets.Get(0, &batch{Idx: 0, Len: 10, Cap: 10}, true)
	assert.Equal(t, 0, begin)
	assert.Equal(t, 0, end)

	begin, end = offsets.Get(0, &batch{Idx: 1, Len: 10, Cap: 10}, true)
	assert.Equal(t, 0, begin)
	assert.Equal(t, 0, end)
}

func TestUnchangedBatchRead(t *testing.T) {
	const numCPUs = 1
	offsets := newOffsetManager(numCPUs)

	begin, end := offsets.Get(0, &batch{Idx: 0, Len: 5, Cap: 10}, true)
	assert.Equal(t, 0, begin)
	assert.Equal(t, 5, end)

	begin, end = offsets.Get(0, &batch{Idx: 0, Len: 5, Cap: 10}, true)
	assert.Equal(t, 5, begin)
	assert.Equal(t, 5, end)
}

func TestReadGap(t *testing.T) {
	const numCPUs = 1
	offsets := newOffsetManager(numCPUs)

	// this emulates the scenario where a batch is lost in the perf buffer
	begin, end := offsets.Get(0, &batch{Idx: 0, Len: 10, Cap: 10}, true)
	assert.Equal(t, 0, begin)
	assert.Equal(t, 10, end)
	assert.Equal(t, 1, offsets.NextBatchID(0))

	// batch idx=1 was lost
	begin, end = offsets.Get(0, &batch{Idx: 2, Len: 10, Cap: 10}, true)
	assert.Equal(t, 0, begin)
	assert.Equal(t, 10, end)
	assert.Equal(t, 3, offsets.NextBatchID(0))
}
