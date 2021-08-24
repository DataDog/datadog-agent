// +build linux_bpf

package kprobe

import (
	"testing"
	"time"
	"unsafe"

	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	numTestCPUs = 4
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

		conns := manager.Extract(batch, 0)
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

		conns := manager.Extract(batch, 0)
		assert.Len(t, conns, 1)
		assert.Equal(t, uint32(4), conns[0].Pid)
	})
}

func TestGetPendingConns(t *testing.T) {
	manager, doneFn := newTestBatchManager(t)
	defer doneFn()

	batch := new(netebpf.Batch)
	batch.Id = 0
	batch.C0.Tup.Pid = 1
	batch.C1.Tup.Pid = 2
	batch.Len = 2

	cpu := 0
	updateBatch := func() {
		err := manager.batchMap.Put(unsafe.Pointer(&cpu), unsafe.Pointer(batch))
		require.NoError(t, err)
	}
	updateBatch()

	pendingConns := manager.GetPendingConns()
	assert.Len(t, pendingConns, 2)
	assert.Equal(t, uint32(1), pendingConns[0].Pid)
	assert.Equal(t, uint32(2), pendingConns[1].Pid)

	// Now let's pretend a new connection was added to the batch on eBPF side
	batch.C2.Tup.Pid = 3
	batch.Len++
	updateBatch()

	// We should now get only the connection that hasn't been processed before
	pendingConns = manager.GetPendingConns()
	assert.Len(t, pendingConns, 1)
	assert.Equal(t, uint32(3), pendingConns[0].Pid)
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

	manager.GetPendingConns()
	_, ok := manager.stateByCPU[cpu].processed[batch.Id]
	require.True(t, ok)
	assert.Equal(t, uint16(2), manager.stateByCPU[cpu].processed[batch.Id].offset)

	manager.cleanupExpiredState(time.Now().Add(manager.expiredStateInterval))
	manager.GetPendingConns()

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
	ctr, err := NewTracer(testConfig(), nil)
	require.NoError(t, err)

	tr := ctr.(*kprobeTracer)
	manager := tr.batchManager
	doneFn := func() { tr.Stop() }
	return manager, doneFn
}
