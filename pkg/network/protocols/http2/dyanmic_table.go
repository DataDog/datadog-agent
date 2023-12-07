// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/hashicorp/golang-lru/v2/simplelru"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/intern"
)

// DynamicTable stores all interesting dynamic values captured by the kernel.
// It is used to overcome the lack of a proper LRU datastore in the kernel, and we are not able to clear room for new
// values, once our kernel maps are full.
// The user mode dynamic table combines two features to achieve better memory usage:
// - a string interner, to avoid storing the same string multiple times
// - an LRU datastore, to evict the least recently used entries when the table is full
//
// The dynamic table also clear entries from the kernel map called `http2_interesting_dynamic_table_set`.
type DynamicTable struct {
	// dynamicTableSize is the size of the dynamic table
	dynamicTableSize int
	// mux is used to protect the dynamic table from concurrent access
	mux sync.RWMutex
	// stopChannel is used to mark our main goroutine to stop
	stopChannel chan struct{}
	// wg is used to wait for the dynamic table to stop
	wg sync.WaitGroup
	// datastore is the LRU datastore used to store the dynamic table entries
	datastore *simplelru.LRU[http2DynamicTableIndex, *intern.StringValue]
	// stringInternStore is the string interner used to avoid storing the same string multiple times
	stringInternStore *intern.StringInterner
	// kernelMap is the kernel map (`http2_interesting_dynamic_table_set`) used to store the dynamic table entries
	kernelMap *ebpf.Map
	// perfHandler is the perf handler used to receive new paths from the kernel
	perfHandler *ddebpf.PerfHandler
}

// NewDynamicTable creates a new dynamic table.
func NewDynamicTable(dynamicTableSize int) *DynamicTable {
	return &DynamicTable{
		dynamicTableSize:  dynamicTableSize,
		stringInternStore: intern.NewStringInterner(),
		perfHandler:       ddebpf.NewPerfHandler(dynamicTableSize),
		stopChannel:       make(chan struct{}),
	}
}

// ResolvePath resolves the path of a given index and connection tuple.
func (dt *DynamicTable) ResolvePath(connTuple netebpf.ConnTuple, index uint64) (*intern.StringValue, bool) {
	dt.mux.RLock()
	defer dt.mux.RUnlock()

	return dt.datastore.Get(http2DynamicTableIndex{
		Index: index,
		Tup:   connTuple,
	})
}

// AddPath adds a new path to the dynamic table and the string interner.
func (dt *DynamicTable) AddPath(key http2DynamicTableIndex, path []byte) {
	dt.mux.Lock()
	dt.datastore.Add(key, dt.stringInternStore.Get(path))
	dt.mux.Unlock()

	value := true
	_ = dt.kernelMap.Update(unsafe.Pointer(&key), unsafe.Pointer(&value), 0)
}

// StartProcessingPerfHandler starts the perf handler used to receive new paths from the kernel.
func (dt *DynamicTable) StartProcessingPerfHandler(mgr *manager.Manager) error {
	kernelMap, ok, err := mgr.GetMap("http2_interesting_dynamic_table_set")
	if err != nil {
		return err
	} else if !ok {
		return errors.New("kernel map http2_interesting_dynamic_table_set not found")
	}
	dt.kernelMap = kernelMap

	lru, err := simplelru.NewLRU[http2DynamicTableIndex, *intern.StringValue](dt.dynamicTableSize, func(key http2DynamicTableIndex, _ *intern.StringValue) {
		_ = kernelMap.Delete(unsafe.Pointer(&key))
	})
	if err != nil {
		return fmt.Errorf("error creating an LRU datastore for http2 dynamic table: %w", err)
	}
	dt.datastore = lru

	dt.wg.Add(1)
	go func() {
		defer dt.wg.Done()
		for {
			select {
			case <-dt.stopChannel:
				return
			case data, ok := <-dt.perfHandler.DataChannel:
				if !ok {
					return
				}
				// TODO: process path
				data.Done()
			case _, ok := <-dt.perfHandler.LostChannel:
				if !ok {
					return
				}
			}
		}
	}()

	return nil
}

// StopProcessingPerfHandler stops the perf handler.
func (dt *DynamicTable) StopProcessingPerfHandler() {
	close(dt.stopChannel)
	dt.wg.Wait()
}
