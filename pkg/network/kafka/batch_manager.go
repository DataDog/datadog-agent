// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

import (
	"errors"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"unsafe"

	"github.com/cilium/ebpf"
)

var errLostBatch = errors.New("kafka batch lost (not consumed fast enough)")

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
	batch := new(kafkaBatch)
	stateByCPU := make([]usrBatchState, numCPUs)

	for i := 0; i < numCPUs; i++ {
		// Initialize eBPF maps
		for j := 0; j < KAFKABatchPages; j++ {
			key := &kafkaBatchKey{Cpu: uint32(i), Num: uint32(j)}
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

func (m *batchManager) GetTransactionsFrom(event *ddebpf.DataEvent) ([]kafkaTX, error) {
	state := &m.stateByCPU[event.CPU]
	batch := batchFromEventData(event.Data)

	if int(batch.Idx) < state.idx {
		// This means this batch was processed via GetPendingTransactions
		return nil, nil
	}

	offset := state.pos
	state.idx = int(batch.Idx) + 1
	state.pos = 0

	txns := make([]kafkaTX, len(batch.Transactions()[offset:]))
	tocopy := batch.Transactions()[offset:]
	for idx := range tocopy {
		txns[idx] = &tocopy[idx]
	}
	return txns, nil
}

func (m *batchManager) GetPendingTransactions() []kafkaTX {
	transactions := make([]kafkaTX, 0, KAFKABatchSize*KAFKABatchPages/2)
	for i := 0; i < m.numCPUs; i++ {
		for lookup := 0; lookup < maxLookupsPerCPU; lookup++ {
			var (
				usrState = &m.stateByCPU[i]
				pageNum  = usrState.idx % KAFKABatchPages
				batchKey = &kafkaBatchKey{Cpu: uint32(i), Num: uint32(pageNum)}
				batch    = new(kafkaBatch)
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

			if krnStatePos == KAFKABatchSize {
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

func batchFromEventData(data []byte) *kafkaBatch {
	return (*kafkaBatch)(unsafe.Pointer(&data[0]))
}
