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

	"fmt"

	"github.com/cilium/ebpf"
)

/*
#include "../ebpf/c/http-types.h"
*/
import "C"

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
			key := &httpBatchKey{cpu: C.uint(i), page_num: C.uint(j)}
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

	if int(batch.idx) < state.idx {
		// This means this batch was processed via GetPendingTransactions
		return nil, nil
	}

	if batch.IsDirty(notification) {
		// This means the batch was overridden before we a got chance to read it
		return nil, errLostBatch
	}

	offset := state.pos
	state.idx = int(notification.batch_idx) + 1
	state.pos = 0

	return batch.Transactions()[offset:], nil
}

func (m *batchManager) GetPendingTransactions() []httpTX {
	transactions := make([]httpTX, 0, HTTPBatchSize*HTTPBatchPages/2)
	for i := 0; i < m.numCPUs; i++ {
		for lookup := 0; lookup < maxLookupsPerCPU; lookup++ {
			var (
				usrState = &m.stateByCPU[i]
				pageNum  = usrState.idx % HTTPBatchPages
				batchKey = &httpBatchKey{cpu: C.uint(i), page_num: C.uint(pageNum)}
				batch    = new(httpBatch)
			)

			err := m.batchMap.Lookup(unsafe.Pointer(batchKey), unsafe.Pointer(batch))
			if err != nil {
				break
			}

			krnStateIDX := int(batch.idx)
			krnStatePos := int(batch.pos)
			if krnStateIDX != usrState.idx || krnStatePos <= usrState.pos {
				break
			}

			all := batch.Transactions()
			pending := all[usrState.pos:krnStatePos]
			transactions = append(transactions, pending...)

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
