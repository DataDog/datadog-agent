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
	numTestBatches = 4
)

func TestPerfBatchManagerExtract(t *testing.T) {
	t.Run("normal flush", func(t *testing.T) {
		manager := PerfBatchManager{stateByCPU: make([]batchState, numTestBatches)}

		batch := new(batch)
		batch.c0.tup.pid = 1
		batch.c1.tup.pid = 2
		batch.c2.tup.pid = 3
		batch.c3.tup.pid = 4
		batch.c4.tup.pid = 5
		batch.cpu = 0

		conns := manager.Extract(batch, time.Now())
		assert.Len(t, conns, 5)
		assert.Equal(t, uint32(1), conns[0].Pid)
		assert.Equal(t, uint32(2), conns[1].Pid)
		assert.Equal(t, uint32(3), conns[2].Pid)
		assert.Equal(t, uint32(4), conns[3].Pid)
		assert.Equal(t, uint32(5), conns[4].Pid)
	})

	t.Run("partial flush", func(t *testing.T) {
		manager := PerfBatchManager{stateByCPU: make([]batchState, numTestBatches)}

		batch := new(batch)
		batch.c0.tup.pid = 1
		batch.c1.tup.pid = 2
		batch.c2.tup.pid = 3
		batch.c3.tup.pid = 4
		batch.c4.tup.pid = 5
		batch.cpu = 0

		// Simulate a partial flush
		manager.stateByCPU[0].offset = 3

		conns := manager.Extract(batch, time.Now())
		assert.Len(t, conns, 2)
		assert.Equal(t, uint32(4), conns[0].Pid)
		assert.Equal(t, uint32(5), conns[1].Pid)
	})
}

func TestGetIdleConns(t *testing.T) {
	manager, doneFn := newTestBatchManager(t)
	defer doneFn()

	batch := new(batch)
	batch.c0.tup.pid = 1
	batch.c1.tup.pid = 2
	batch.pos = 2
	batch.cpu = 0

	updateBatch := func() {
		manager.batchMap.Put(unsafe.Pointer(&batch.cpu), unsafe.Pointer(batch))
	}
	updateBatch()

	idleConns := manager.GetIdleConns()
	assert.Len(t, idleConns, 2)
	assert.Equal(t, uint32(1), idleConns[0].Pid)
	assert.Equal(t, uint32(2), idleConns[1].Pid)

	// Now let's pretend a new connection was added to the batch on eBPF side
	batch.c2.tup.pid = 3
	batch.pos++
	updateBatch()

	// We should now get only the connection that hasn't been processed before
	idleConns = manager.GetIdleConns()
	assert.Len(t, idleConns, 1)
	assert.Equal(t, uint32(3), idleConns[0].Pid)
}

func newTestBatchManager(t *testing.T) (manager *PerfBatchManager, doneFn func()) {
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)

	connCloseMap, _ := tr.getMap(probes.ConnCloseBatchMap)
	manager, err = NewPerfBatchManager(connCloseMap, numTestBatches)
	require.NoError(t, err)

	doneFn = func() { tr.Stop() }
	return manager, doneFn
}
