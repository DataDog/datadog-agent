// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"errors"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"

	"github.com/cilium/ebpf"
)

var errLostBatch = errors.New("http batch lost (not consumed fast enough)")

const maxLookupsPerCPU = 2

type usrBatchState struct {
	idx, pos int
}

type batchManager struct {
	batchMap   *ebpf.Map
	stateByCPU []usrBatchState
	numCPUs    int
}

func newBatchManager(batchMap *ebpf.Map, numCPUs int) (*batchManager, error) {
	batch := new(httpBatch)
	stateByCPU := make([]usrBatchState, numCPUs)

	for i := 0; i < numCPUs; i++ {
		// Initialize eBPF maps
		for j := 0; j < HTTPBatchPages; j++ {
			key := &httpBatchKey{Cpu: uint32(i), Num: uint32(j)}
			err := batchMap.Put(unsafe.Pointer(key), unsafe.Pointer(batch))
			if err != nil {
				return nil, err
			}
		}
	}

	return &batchManager{
		batchMap:   batchMap,
		stateByCPU: stateByCPU,
		numCPUs:    numCPUs,
	}, nil
}

func (m *batchManager) GetTransactionsFrom(event *ddebpf.DataEvent) ([]transaction.HttpTX, error) {
	state := &m.stateByCPU[event.CPU]
	batch := batchFromEventData(event.Data)

	if int(batch.Idx) < state.idx {
		// This means this batch was processed via GetPendingTransactions
		return nil, nil
	}

	offset := state.pos
	state.idx = int(batch.Idx) + 1
	state.pos = 0

	txns := make([]transaction.HttpTX, len(batch.Transactions()[offset:]))
	tocopy := batch.Transactions()[offset:]
	for idx := range tocopy {
		txns[idx] = &tocopy[idx]
	}
	return txns, nil
}

func (m *batchManager) GetPendingTransactions() []transaction.HttpTX {
	transactions := make([]httpTX, 0, HTTPBatchSize*HTTPBatchPages/2)
	for i := 0; i < m.numCPUs; i++ {
		for lookup := 0; lookup < maxLookupsPerCPU; lookup++ {
			var (
				usrState = &m.stateByCPU[i]
				pageNum  = usrState.idx % HTTPBatchPages
				batchKey = &httpBatchKey{Cpu: uint32(i), Num: uint32(pageNum)}
				batch    = new(httpBatch)
			)

			err := m.batchMap.Lookup(unsafe.Pointer(batchKey), unsafe.Pointer(batch))
			if err != nil {
				break
			}

			krnStateIDX := int(batch.Idx)
			krnStatePos := int(batch.Pos)
			if krnStateIDX != usrState.idx || krnStatePos <= usrState.pos {
				break
			}

			all := batch.Transactions()
			pending := all[usrState.pos:krnStatePos]
			for _, tx := range pending {
				var newtx = tx
				transactions = append(transactions, &newtx)
			}

			if krnStatePos == HTTPBatchSize {
				// We detected a full batch before the http_notification_t was processed.
				// In this case we update the userspace state accordingly and try to
				// preemptively read the next batch in order to return as many
				// completed HTTP transactions as possible
				usrState.idx++
				usrState.pos = 0
				continue
			}

			usrState.pos = krnStatePos
			// Move on to the next CPU core
			break
		}
	}

	return transactions
}

func batchFromEventData(data []byte) *httpBatch {
	return (*httpBatch)(unsafe.Pointer(&data[0]))
}

// Transactions returns the slice of HTTP transactions embedded in the batch
func (batch *httpBatch) Transactions() []transaction.EbpfHttpTx {
	return (*(*[netebpf.HTTPBatchSize]transaction.EbpfHttpTx)(unsafe.Pointer(&batch.txs)))[:]
}
