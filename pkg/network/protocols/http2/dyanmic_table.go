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
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
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
	// terminatedConnectionsEventsConsumer is the consumer used to receive terminated connections events from the kernel.
	terminatedConnectionsEventsConsumer *events.Consumer[netebpf.ConnTuple]
	// terminatedConnections is the list of terminated connections received from the kernel.
	terminatedConnections []netebpf.ConnTuple
	// terminatedConnectionMux is used to protect the terminated connections list from concurrent access.
	terminatedConnectionMux sync.Mutex
	// mapCleaner is the map cleaner used to clear entries of terminated connections from the kernel map.
	mapCleaner *ddebpf.MapCleaner[http2DynamicTableIndex, bool]
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

func (dt *DynamicTable) Start(mgr *manager.Manager, cfg *config.Config) (err error) {
	dt.terminatedConnectionsEventsConsumer, err = events.NewConsumer[netebpf.ConnTuple](
		terminatedConnectionsEventStream,
		mgr,
		dt.processTerminatedConnections,
	)
	if err != nil {
		return
	}

	dt.terminatedConnectionsEventsConsumer.Start()

	if err := dt.StartProcessingPerfHandler(mgr); err != nil {
		return err
	}
	return dt.setupDynamicTableMapCleaner(cfg)
}

func (dt *DynamicTable) processTerminatedConnections(events []netebpf.ConnTuple) {
	dt.terminatedConnectionMux.Lock()
	defer dt.terminatedConnectionMux.Unlock()
	dt.terminatedConnections = append(dt.terminatedConnections, events...)
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
		dummyValue := true
		defer dt.wg.Done()
		for {
			select {
			case <-dt.stopChannel:
				return
			case data, ok := <-dt.perfHandler.DataChannel:
				if !ok {
					return
				}
				val := (*http2DynamicTableValue)(unsafe.Pointer(&data.Data[0]))
				res, err := decodeHTTP2Path(val.Buf, val.Len)
				if err == nil {
					// Adding the new path to the dynamic table and the string interner.
					// This may trigger an eviction in the LRU datastore, and maybe remove the evicted entry from the kernel map.
					dt.datastore.Add(val.Key, dt.stringInternStore.Get(res))
					// Although it is done by the kernel as well, the kernel may fail if the map is full (eviction happens in userspace),
					// thus, we do it here as well to avoid losing entries.
					// We're ignoring the error as we're trying to do best-effort here.
					_ = dt.kernelMap.Update(unsafe.Pointer(&val.Key), unsafe.Pointer(&dummyValue), 0)
				}
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

func (dt *DynamicTable) setupDynamicTableMapCleaner(cfg *config.Config) error {
	mapCleaner, err := ddebpf.NewMapCleaner[http2DynamicTableIndex, bool](dt.kernelMap, 1024)
	if err != nil {
		return fmt.Errorf("error creating a map cleaner for http2 dynamic table: %w", err)
	}

	terminatedConnectionsMap := make(map[netebpf.ConnTuple]struct{})
	mapCleaner.Clean(cfg.HTTP2DynamicTableMapCleanerInterval,
		func() bool {
			dt.terminatedConnectionsEventsConsumer.Sync()
			dt.terminatedConnectionMux.Lock()
			for _, conn := range dt.terminatedConnections {
				terminatedConnectionsMap[conn] = struct{}{}
			}
			dt.terminatedConnections = dt.terminatedConnections[:0]
			dt.terminatedConnectionMux.Unlock()

			return len(terminatedConnectionsMap) > 0
		},
		func() {
			terminatedConnectionsMap = make(map[netebpf.ConnTuple]struct{})
		},
		func(_ int64, key http2DynamicTableIndex, _ bool) bool {
			_, ok := terminatedConnectionsMap[key.Tup]
			return ok
		})
	dt.mapCleaner = mapCleaner
	return nil
}

// Stop stops the perf handler.
func (dt *DynamicTable) Stop() {
	close(dt.stopChannel)
	dt.wg.Wait()

	dt.mapCleaner.Stop()

	if dt.terminatedConnectionsEventsConsumer != nil {
		dt.terminatedConnectionsEventsConsumer.Stop()
	}
}
