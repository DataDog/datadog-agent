// +build linux_bpf

package tracer

import (
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	numTestCPUs = 4
)

func TestPerfBatchManagerExtract(t *testing.T) {
	t.Run("normal flush", func(t *testing.T) {
		manager := newEmptyBatchManager()

		batch := new(batch)
		batch.id = 0
		batch.c0.tup.pid = 1
		batch.c1.tup.pid = 2
		batch.c2.tup.pid = 3
		batch.c3.tup.pid = 4

		conns := manager.Extract(batch, 0)
		assert.Len(t, conns, 4)
		assert.Equal(t, uint32(1), conns[0].Pid)
		assert.Equal(t, uint32(2), conns[1].Pid)
		assert.Equal(t, uint32(3), conns[2].Pid)
		assert.Equal(t, uint32(4), conns[3].Pid)
	})

	t.Run("partial flush", func(t *testing.T) {
		manager := newEmptyBatchManager()

		batch := new(batch)
		batch.id = 0
		batch.c0.tup.pid = 1
		batch.c1.tup.pid = 2
		batch.c2.tup.pid = 3
		batch.c3.tup.pid = 4

		// Simulate a partial flush
		manager.stateByCPU[0].processed = map[uint64]batchState{
			0: {offset: 3},
		}

		conns := manager.Extract(batch, 0)
		assert.Len(t, conns, 1)
		assert.Equal(t, uint32(4), conns[0].Pid)
	})
}

func TestGetIdleConns(t *testing.T) {
	manager, doneFn := newTestBatchManager(t)
	defer doneFn()

	batch := new(batch)
	batch.id = 0
	batch.c0.tup.pid = 1
	batch.c1.tup.pid = 2
	batch.len = 2

	cpu := 0
	updateBatch := func() {
		manager.batchMap.Put(unsafe.Pointer(&cpu), unsafe.Pointer(batch))
	}
	updateBatch()

	idleConns := manager.GetIdleConns()
	assert.Len(t, idleConns, 2)
	assert.Equal(t, uint32(1), idleConns[0].Pid)
	assert.Equal(t, uint32(2), idleConns[1].Pid)

	// Now let's pretend a new connection was added to the batch on eBPF side
	batch.c2.tup.pid = 3
	batch.len++
	updateBatch()

	// We should now get only the connection that hasn't been processed before
	idleConns = manager.GetIdleConns()
	assert.Len(t, idleConns, 1)
	assert.Equal(t, uint32(3), idleConns[0].Pid)
}

func TestPerfBatchStateCleanup(t *testing.T) {
	manager, doneFn := newTestBatchManager(t)
	defer doneFn()
	manager.expiredStateInterval = 100 * time.Millisecond

	batch := new(batch)
	batch.id = 0
	batch.c0.tup.pid = 1
	batch.c1.tup.pid = 2
	batch.len = 2

	cpu := 0
	manager.batchMap.Put(unsafe.Pointer(&cpu), unsafe.Pointer(batch))

	manager.GetIdleConns()
	_, ok := manager.stateByCPU[cpu].processed[uint64(batch.id)]
	require.True(t, ok)
	assert.Equal(t, uint16(2), manager.stateByCPU[cpu].processed[uint64(batch.id)].offset)

	manager.cleanupExpiredState(time.Now().Add(manager.expiredStateInterval))
	manager.GetIdleConns()

	// state should not have been cleaned up, since no more connections have happened
	_, ok = manager.stateByCPU[cpu].processed[uint64(batch.id)]
	require.True(t, ok)
	assert.Equal(t, uint16(2), manager.stateByCPU[cpu].processed[uint64(batch.id)].offset)
}

func newEmptyBatchManager() *PerfBatchManager {
	p := PerfBatchManager{stateByCPU: make([]percpuState, numTestCPUs)}
	for cpu := 0; cpu < numTestCPUs; cpu++ {
		p.stateByCPU[cpu] = percpuState{processed: make(map[uint64]batchState)}
	}
	return &p
}

func newTestBatchManager(t *testing.T) (manager *PerfBatchManager, doneFn func()) {
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)

	connCloseMap, _ := tr.getMap(probes.ConnCloseBatchMap)
	manager, err = NewPerfBatchManager(connCloseMap, numTestCPUs)
	require.NoError(t, err)

	doneFn = func() { tr.Stop() }
	return manager, doneFn
}
