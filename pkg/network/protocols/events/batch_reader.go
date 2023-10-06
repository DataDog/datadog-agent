// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package events

import (
	"sync"
	"unsafe"

	"github.com/cilium/ebpf"
)

var batchPool = sync.Pool{
	New: func() interface{} {
		return new(batch)
	},
}

type batchReader struct {
	sync.Mutex
	numCPUs    int
	batchMap   *ebpf.Map
	offsets    *offsetManager
	workerPool *workerPool
	stopped    bool
}

func newBatchReader(offsetManager *offsetManager, batchMap *ebpf.Map, numCPUs int) (*batchReader, error) {
	// initialize eBPF maps
	batch := new(batch)
	for i := 0; i < numCPUs; i++ {
		for j := 0; j < batchPagesPerCPU; j++ {
			key := &batchKey{Cpu: uint32(i), Num: uint32(j)}
			err := batchMap.Put(unsafe.Pointer(key), unsafe.Pointer(batch))
			if err != nil {
				return nil, err
			}
		}
	}

	workerPool, err := newWorkerPool(max(numCPUs, 32))
	if err != nil {
		return nil, err
	}

	return &batchReader{
		numCPUs:    numCPUs,
		offsets:    offsetManager,
		batchMap:   batchMap,
		workerPool: workerPool,
	}, nil
}

// ReadAll batches from eBPF (concurrently) and execute the given
// callback function for each batch
func (r *batchReader) ReadAll(f func(cpu int, b *batch)) {
	// This lock is used only for the purposes of synchronizing termination
	// and it's only held while *enqueing* the jobs.
	r.Lock()
	if r.stopped {
		r.Unlock()
		return
	}

	var wg sync.WaitGroup
	wg.Add(r.numCPUs)

	for i := 0; i < r.numCPUs; i++ {
		cpu := i // required to properly capture this variable in the function closure
		r.workerPool.Do(func() {
			defer wg.Done()
			batchID, key := r.generateBatchKey(cpu)

			b := batchPool.Get().(*batch)
			defer func() {
				*b = batch{}
				batchPool.Put(b)
			}()

			err := r.batchMap.Lookup(unsafe.Pointer(key), unsafe.Pointer(b))
			if err != nil {
				return
			}
			if int(b.Idx) != batchID {
				// this means that the batch we were looking for was flushed to the perf buffer
				return
			}

			f(cpu, b)
		})
	}
	r.Unlock()
	wg.Wait()
}

// Stop batchReader
func (r *batchReader) Stop() {
	r.Lock()
	defer r.Unlock()

	if r.stopped {
		return
	}
	r.stopped = true
	r.workerPool.Stop()
}

func (r *batchReader) generateBatchKey(cpu int) (batchID int, key *batchKey) {
	batchID = r.offsets.NextBatchID(cpu)
	return batchID, &batchKey{
		Cpu: uint32(cpu),
		Num: uint32(batchID) % uint32(batchPagesPerCPU),
	}
}
