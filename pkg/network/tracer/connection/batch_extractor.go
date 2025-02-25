// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"time"

	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
)

const defaultExpiredStateInterval = 60 * time.Second

type batchExtractor struct {
	numCPUs int
	// stateByCPU contains the state of each batch.
	// The slice is indexed by the CPU core number.
	stateByCPU           []percpuState
	expiredStateInterval time.Duration
}

type percpuState struct {
	// map of batch id -> offset of conns already processed by GetPendingConns
	processed map[uint64]batchState
}

type batchState struct {
	offset  uint16
	updated time.Time
}

func newBatchExtractor(numCPUs int) *batchExtractor {
	state := make([]percpuState, numCPUs)
	for cpu := 0; cpu < numCPUs; cpu++ {
		state[cpu] = percpuState{
			processed: make(map[uint64]batchState),
		}
	}
	return &batchExtractor{
		numCPUs:              numCPUs,
		stateByCPU:           state,
		expiredStateInterval: defaultExpiredStateInterval,
	}
}

// NumCPUs returns the number of CPUs the batch extractor has been initialized for
func (e *batchExtractor) NumCPUs() int {
	return e.numCPUs
}

// NextConnection returns the next unprocessed connection from the batch.
// Returns nil if no more connections are left.
func (e *batchExtractor) NextConnection(b *netebpf.Batch) *netebpf.Conn {
	cpu := int(b.Cpu)
	if cpu >= e.numCPUs {
		return nil
	}
	if b.Len == 0 {
		return nil
	}

	batchID := b.Id
	cpuState := &e.stateByCPU[cpu]
	offset := uint16(0)
	if bState, ok := cpuState.processed[batchID]; ok {
		offset = bState.offset
		if offset >= netebpf.BatchSize {
			delete(cpuState.processed, batchID)
			return nil
		}
		if offset >= b.Len {
			return nil
		}
	}

	defer func() {
		cpuState.processed[batchID] = batchState{
			offset:  offset + 1,
			updated: time.Now(),
		}
	}()

	switch offset {
	case 0:
		return &b.C0
	case 1:
		return &b.C1
	case 2:
		return &b.C2
	case 3:
		return &b.C3
	default:
		panic("batch size is out of sync")
	}
}

// CleanupExpiredState removes entries from per-cpu state that haven't been updated in the last minute
func (e *batchExtractor) CleanupExpiredState(now time.Time) {
	for cpu := 0; cpu < len(e.stateByCPU); cpu++ {
		cpuState := &e.stateByCPU[cpu]
		for id, s := range cpuState.processed {
			if now.Sub(s.updated) >= e.expiredStateInterval {
				delete(cpuState.processed, id)
			}
		}
	}
}
