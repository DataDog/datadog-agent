// +build linux_bpf

package ebpf

import (
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerfBatchManagerExtract(t *testing.T) {
	t.Run("normal flush", func(t *testing.T) {
		manager := PerfBatchManager{stateByCPU: make([]batchState, maxNumberBatches)}

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
		manager := PerfBatchManager{stateByCPU: make([]batchState, maxNumberBatches)}

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

func TestGetIdleConnsZeroState(t *testing.T) {
	const maxIdleTime = 10 * time.Second
	manager, doneFn := newTestBatchManager(t, maxIdleTime)
	defer doneFn()
	assert.Len(t, manager.GetIdleConns(time.Now()), 0)
}

func TestGetIdleConns(t *testing.T) {
	const maxIdleTime = 10 * time.Second
	manager, doneFn := newTestBatchManager(t, maxIdleTime)
	defer doneFn()

	batch := new(batch)
	batch.c0.tup.pid = 1
	batch.c1.tup.pid = 2
	batch.pos = 2
	batch.cpu = 0

	updateBatch := func() {
		manager.module.UpdateElement(
			manager.batchMap,
			unsafe.Pointer(&batch.cpu),
			unsafe.Pointer(batch),
			0,
		)
	}
	updateBatch()

	now := time.Now()
	idleConns := manager.GetIdleConns(now)
	assert.Len(t, idleConns, 2)
	assert.Equal(t, uint32(1), idleConns[0].Pid)
	assert.Equal(t, uint32(2), idleConns[1].Pid)

	// Now let's pretend a new connection was added to the batch on eBPF side
	batch.c2.tup.pid = 3
	batch.pos++
	updateBatch()

	// We should not get anything back since 5 seconds < idleTime (10 seconds)
	idleConns = manager.GetIdleConns(now.Add(5 * time.Second))
	assert.Len(t, idleConns, 0)

	// We should now get only the connection that hasn't been processed before
	idleConns = manager.GetIdleConns(now.Add(15 * time.Second))
	assert.Len(t, idleConns, 1)
	assert.Equal(t, uint32(3), idleConns[0].Pid)
}

func newTestBatchManager(t *testing.T, idleTime time.Duration) (manager *PerfBatchManager, doneFn func()) {
	tr, err := NewTracer(NewDefaultConfig())
	require.NoError(t, err)

	tcpCloseMap, _ := tr.getMap(tcpCloseBatchMap)
	manager, err = NewPerfBatchManager(tr.m, tcpCloseMap, idleTime)
	require.NoError(t, err)

	doneFn = func() { tr.Stop() }
	return manager, doneFn
}
