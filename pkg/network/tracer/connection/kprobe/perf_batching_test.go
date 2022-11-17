// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kprobe

import (
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
)

const (
	numTestCPUs        = 4
	pidMax      uint32 = 1 << 22 // PID_MAX_LIMIT on 64 bit systems
)

func TestPerfBatchManagerExtract(t *testing.T) {
	t.Run("normal flush", func(t *testing.T) {
		manager := newEmptyBatchManager()

		batch := new(netebpf.Batch)
		batch.Id = 0
		batch.C0.Tup.Pid = 1
		batch.C1.Tup.Pid = 2
		batch.C2.Tup.Pid = 3
		batch.C3.Tup.Pid = 4

		buffer := network.NewConnectionBuffer(256, 256)
		manager.ExtractBatchInto(buffer, batch, 0)
		conns := buffer.Connections()
		assert.Len(t, conns, 4)
		assert.Equal(t, uint32(1), conns[0].Pid)
		assert.Equal(t, uint32(2), conns[1].Pid)
		assert.Equal(t, uint32(3), conns[2].Pid)
		assert.Equal(t, uint32(4), conns[3].Pid)
	})

	t.Run("partial flush", func(t *testing.T) {
		manager := newEmptyBatchManager()

		batch := new(netebpf.Batch)
		batch.Id = 0
		batch.C0.Tup.Pid = 1
		batch.C1.Tup.Pid = 2
		batch.C2.Tup.Pid = 3
		batch.C3.Tup.Pid = 4

		// Simulate a partial flush
		manager.stateByCPU[0].processed = map[uint64]batchState{
			0: {offset: 3},
		}

		buffer := network.NewConnectionBuffer(256, 256)
		manager.ExtractBatchInto(buffer, batch, 0)
		conns := buffer.Connections()
		assert.Len(t, conns, 1)
		assert.Equal(t, uint32(4), conns[0].Pid)
	})
}

func TestGetPendingConns(t *testing.T) {
	manager, doneFn := newTestBatchManager(t)
	defer doneFn()

	batch := new(netebpf.Batch)
	batch.Id = 0
	batch.C0.Tup.Pid = pidMax + 1
	batch.C1.Tup.Pid = pidMax + 2
	batch.Len = 2

	cpu := 0
	updateBatch := func() {
		err := manager.batchMap.Put(unsafe.Pointer(&cpu), unsafe.Pointer(batch))
		require.NoError(t, err)
	}
	updateBatch()

	buffer := network.NewConnectionBuffer(256, 256)
	manager.GetPendingConns(buffer)
	pendingConns := buffer.Connections()
	assert.GreaterOrEqual(t, len(pendingConns), 2)
	for _, pid := range []uint32{pidMax + 1, pidMax + 2} {
		found := false
		for p := range pendingConns {
			if pendingConns[p].Pid == pid {
				found = true
				pendingConns = append(pendingConns[:p], pendingConns[p+1:]...)
				break
			}
		}

		assert.True(t, found, "could not find batched connection for pid %d", pid)
	}

	// Now let's pretend a new connection was added to the batch on eBPF side
	batch.C2.Tup.Pid = pidMax + 3
	batch.Len++
	updateBatch()

	// We should now get only the connection that hasn't been processed before
	buffer.Reset()
	manager.GetPendingConns(buffer)
	pendingConns = buffer.Connections()
	assert.GreaterOrEqual(t, len(pendingConns), 1)
	var found bool
	for _, p := range pendingConns {
		if p.Pid == pidMax+3 {
			found = true
			break
		}
	}

	assert.True(t, found, "could not find batched connection for pid %d", pidMax+3)
}

func TestPerfBatchStateCleanup(t *testing.T) {
	manager, doneFn := newTestBatchManager(t)
	defer doneFn()
	manager.expiredStateInterval = 100 * time.Millisecond

	batch := new(netebpf.Batch)
	batch.Id = 0
	batch.C0.Tup.Pid = 1
	batch.C1.Tup.Pid = 2
	batch.Len = 2

	cpu := 0
	err := manager.batchMap.Put(unsafe.Pointer(&cpu), unsafe.Pointer(batch))
	require.NoError(t, err)

	buffer := network.NewConnectionBuffer(256, 256)
	manager.GetPendingConns(buffer)
	_, ok := manager.stateByCPU[cpu].processed[batch.Id]
	require.True(t, ok)
	assert.Equal(t, uint16(2), manager.stateByCPU[cpu].processed[batch.Id].offset)

	manager.cleanupExpiredState(time.Now().Add(manager.expiredStateInterval))
	manager.GetPendingConns(buffer)

	// state should not have been cleaned up, since no more connections have happened
	_, ok = manager.stateByCPU[cpu].processed[batch.Id]
	require.True(t, ok)
	assert.Equal(t, uint16(2), manager.stateByCPU[cpu].processed[batch.Id].offset)
}

func newEmptyBatchManager() *perfBatchManager {
	p := perfBatchManager{stateByCPU: make([]percpuState, numTestCPUs)}
	for cpu := 0; cpu < numTestCPUs; cpu++ {
		p.stateByCPU[cpu] = percpuState{processed: make(map[uint64]batchState)}
	}
	return &p
}

func newTestBatchManager(t *testing.T) (*perfBatchManager, func()) {
	ctr, err := New(testConfig(), nil, nil)
	require.NoError(t, err)

	tr := ctr.(*kprobeTracer)
	// do not start tracer, so we don't pick up on any connections outside the test

	manager := tr.closeConsumer.batchManager
	doneFn := func() { tr.Stop() }
	return manager, doneFn
}
