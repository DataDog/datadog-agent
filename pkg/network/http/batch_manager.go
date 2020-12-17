// +build linux_bpf

package http

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/ebpf"

	"C"
)

type batchManager struct {
	batchMap   *ebpf.Map
	stateByCPU []httpBatchState
	numCPUs    int

	// telemetry
	misses int
}

func newBatchManager(batchMap, batchStateMap *ebpf.Map, numCPUs int) *batchManager {
	batch := new(httpBatch)
	batchState := new(httpBatchState)
	stateByCPU := make([]httpBatchState, numCPUs)

	for i := 0; i < numCPUs; i++ {
		// Initialize eBPF maps
		batchStateMap.Put(unsafe.Pointer(&i), unsafe.Pointer(batchState))
		for j := 0; j < HTTPBatchPages; j++ {
			key := &httpBatchKey{cpu: C.uint(i), page_num: C.uint(j)}
			batchMap.Put(unsafe.Pointer(key), unsafe.Pointer(batch))
		}
	}

	return &batchManager{
		batchMap:   batchMap,
		stateByCPU: stateByCPU,
		numCPUs:    numCPUs,
	}
}

func (m *batchManager) GetTransactionsFrom(notification httpNotification) []httpTX {
	state := &m.stateByCPU[notification.cpu]

	batch := new(httpBatch)
	key := new(httpBatchKey)
	key.Prepare(notification)
	err := m.batchMap.Lookup(unsafe.Pointer(key), unsafe.Pointer(batch))
	if err != nil {
		log.Errorf("error retrieving http batch for cpu=%d", notification.cpu)
		return nil
	}

	if batch.IsDirty(notification) {
		m.misses++
		return nil
	}

	offset := state.pos
	state.idx = notification.batch_idx + 1
	state.pos = 0

	return batch.Transactions()[offset:]
}

func (m *batchManager) GetPendingTransactions() []httpTX {
	transactions := make([]httpTX, 0, HTTPBatchSize*HTTPBatchPages)

	for i := 0; i < m.numCPUs; i++ {
		state := &m.stateByCPU[i]
		page := int(state.idx) % HTTPBatchPages
		key := &httpBatchKey{cpu: C.uint(i), page_num: C.uint(page)}
		batch := new(httpBatch)

		m.batchMap.Lookup(unsafe.Pointer(key), unsafe.Pointer(batch))
		if batch.state.idx != state.idx || batch.state.pos <= state.pos || int(batch.state.pos) >= HTTPBatchSize {
			continue
		}

		all := batch.Transactions()
		pending := all[int(state.pos):int(batch.state.pos)]
		transactions = append(transactions, pending...)
		m.stateByCPU[i].pos = batch.state.pos
	}

	return transactions
}
