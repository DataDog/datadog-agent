// +build linux_bpf

package http

import (
	"errors"
	"unsafe"

	"C"
	"fmt"

	"github.com/DataDog/ebpf"
)

var errLostBatch = errors.New("http batch lost (not consumed fast enough)")

type batchManager struct {
	batchMap   *ebpf.Map
	stateByCPU []httpBatchState
	numCPUs    int
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

func (m *batchManager) GetTransactionsFrom(notification httpNotification) ([]httpTX, error) {
	var (
		state    = &m.stateByCPU[notification.cpu]
		batch    = new(httpBatch)
		batchKey = new(httpBatchKey)
	)

	batchKey.Prepare(notification)
	err := m.batchMap.Lookup(unsafe.Pointer(batchKey), unsafe.Pointer(batch))
	if err != nil {
		return nil, fmt.Errorf("error retrieving http batch for cpu=%d", notification.cpu)
	}

	if batch.IsDirty(notification) {
		return nil, errLostBatch
	}

	offset := state.pos
	state.idx = notification.batch_idx + 1
	state.pos = 0

	return batch.Transactions()[offset:], nil
}

func (m *batchManager) GetPendingTransactions() []httpTX {
	transactions := make([]httpTX, 0, HTTPBatchSize*HTTPBatchPages/2)
	for i := 0; i < m.numCPUs; i++ {
		var (
			usrState = &m.stateByCPU[i]
			pageNum  = int(usrState.idx) % HTTPBatchPages
			batchKey = &httpBatchKey{cpu: C.uint(i), page_num: C.uint(pageNum)}
			batch    = new(httpBatch)
		)

		err := m.batchMap.Lookup(unsafe.Pointer(batchKey), unsafe.Pointer(batch))
		if err != nil {
			continue
		}

		krnState := batch.state
		if krnState.idx != usrState.idx || krnState.pos <= usrState.pos {
			continue
		}

		all := batch.Transactions()
		pending := all[int(usrState.pos):int(krnState.pos)]
		transactions = append(transactions, pending...)
		m.stateByCPU[i].pos = krnState.pos
	}

	return transactions
}
