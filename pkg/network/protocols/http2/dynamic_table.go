// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"fmt"
	"math"
	"os"
	"slices"
	"sync"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"go.uber.org/atomic"
	"golang.org/x/net/http2/hpack"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	terminatedConnectionsEventStream = "terminated_http2"

	defaultMapCleanerBatchSize = 1024

	// We want to ensure the user-mode dynamic table is slightly smaller than the kernel one, to ensure we'll never
	// have a fully filled kernel map. When the user mode map will be full, we'll start evicting entries from the user
	// mode map, and the kernel map will be updated accordingly. Thus, keeping a small buffer in the user mode map
	// ensures we'll never fail to insert a new entry in the kernel map.
	dynamicTableDefaultBufferFactor = 0.9
)

// DynamicTable encapsulates the management of the dynamic table in the user mode.
type DynamicTable struct {
	// wg is used to wait for the dynamic table to stop
	wg sync.WaitGroup
	// mux is used to protect the dynamic table from concurrent access
	mux sync.RWMutex
	// stopChannel is used to mark our main goroutine to stop
	stopChannel chan struct{}
	// perfHandler is the perf handler used to receive dynamic table entries from the kernel
	perfHandler *ddebpf.PerfHandler
	// dynamicTableSize is the size of the dynamic table
	userModeDynamicTableSize int
	// datastore is the datastore used to store the dynamic table entries
	datastore map[netebpf.ConnTuple]*CyclicMap[uint64, string]
	// datastoreSize is the size of the datastore
	datastoreSize atomic.Int64
	// temporaryDatastore is the datastore used to store the dynamic table entries
	temporaryDatastore map[netebpf.ConnTuple]*CyclicMap[uint64, string]
	// temporaryDatastoreSize is the size of the temporaryDatastore
	temporaryDatastoreSize atomic.Int64
	// terminatedConnectionsEventsConsumer is the consumer used to receive terminated connections events from the kernel.
	terminatedConnectionsEventsConsumer *events.Consumer[netebpf.ConnTuple]
	// terminatedConnections is the list of terminated connections received from the kernel.
	terminatedConnections []netebpf.ConnTuple
	// terminatedConnectionMux is used to protect the terminated connections list from concurrent access.
	terminatedConnectionMux sync.Mutex
	// mapCleaner is the map cleaner used to clear entries of terminated connections from the kernel map.
	mapCleaner *ddebpf.MapCleaner[HTTP2DynamicTableIndex, uint32]
}

// NewDynamicTable creates a new dynamic table.
func NewDynamicTable(dynamicTableSize int) *DynamicTable {
	// We want to ensure the user-mode dynamic table is slightly smaller than the kernel one, to ensure we'll never
	// have a fully filled kernel map. When the user mode map will be full, we'll start evicting entries from the user
	// mode map, and the kernel map will be updated accordingly. Thus, keeping a small buffer in the user mode map
	// ensures we'll never fail to insert a new entry in the kernel map.
	return &DynamicTable{
		userModeDynamicTableSize: int(math.Ceil(float64(dynamicTableSize) * dynamicTableDefaultBufferFactor)),
		perfHandler:              ddebpf.NewPerfHandler(dynamicTableSize),
		stopChannel:              make(chan struct{}),
		datastore:                make(map[netebpf.ConnTuple]*CyclicMap[uint64, string], 0),
		temporaryDatastore:       make(map[netebpf.ConnTuple]*CyclicMap[uint64, string], 0),
	}
}

// configureOptions configures the perf handler options for the map cleaner.
func (dt *DynamicTable) configureOptions(mgr *manager.Manager, opts *manager.Options) {
	events.Configure(terminatedConnectionsEventStream, mgr, opts)

	if !slices.ContainsFunc(mgr.PerfMaps, func(p *manager.PerfMap) bool { return p.Map.Name == http2DynamicTablePerfBuffer }) {
		mgr.PerfMaps = append(mgr.PerfMaps, &manager.PerfMap{
			Map: manager.Map{Name: http2DynamicTablePerfBuffer},
			PerfMapOptions: manager.PerfMapOptions{
				PerfRingBufferSize: 16 * os.Getpagesize(),
				Watermark:          1,
				RecordHandler:      dt.perfHandler.RecordHandler,
				LostHandler:        dt.perfHandler.LostHandler,
				RecordGetter:       dt.perfHandler.RecordGetter,
			},
		})
	}
}

// preStart sets up the terminated connections events consumer.
func (dt *DynamicTable) preStart(mgr *manager.Manager) (err error) {
	dt.terminatedConnectionsEventsConsumer, err = events.NewConsumer(
		terminatedConnectionsEventStream,
		mgr,
		dt.processTerminatedConnections,
	)
	if err != nil {
		return
	}

	dt.terminatedConnectionsEventsConsumer.Start()

	return dt.launchPerfHandlerProcessor()
}

// postStart sets up the dynamic table map cleaner.
func (dt *DynamicTable) postStart(mgr *manager.Manager, cfg *config.Config) error {
	return dt.setupDynamicTableMapCleaner(mgr, cfg)
}

// processTerminatedConnections processes the terminated connections received from the kernel.
func (dt *DynamicTable) processTerminatedConnections(events []netebpf.ConnTuple) {
	dt.terminatedConnectionMux.Lock()
	defer dt.terminatedConnectionMux.Unlock()
	dt.terminatedConnections = append(dt.terminatedConnections, events...)
}

// setupDynamicTableMapCleaner sets up the map cleaner used to clear entries of terminated connections from the kernel map.
func (dt *DynamicTable) setupDynamicTableMapCleaner(mgr *manager.Manager, cfg *config.Config) error {
	dynamicTableMap, _, err := mgr.GetMap(dynamicTable)
	if err != nil {
		return fmt.Errorf("error getting http2 dynamic table map: %w", err)
	}

	mapCleaner, err := ddebpf.NewMapCleaner[HTTP2DynamicTableIndex, uint32](dynamicTableMap, defaultMapCleanerBatchSize)
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
			dt.mux.Lock()
			for conn := range terminatedConnectionsMap {
				if _, ok := dt.datastore[conn]; ok {
					dt.datastoreSize.Sub(int64(dt.datastore[conn].Len()))
					delete(dt.datastore, conn)
				}
				if _, ok := dt.temporaryDatastore[conn]; ok {
					dt.temporaryDatastoreSize.Sub(int64(dt.temporaryDatastore[conn].Len()))
					delete(dt.temporaryDatastore, conn)
				}
			}
			dt.mux.Unlock()
			terminatedConnectionsMap = make(map[netebpf.ConnTuple]struct{})
		},
		func(_ int64, key HTTP2DynamicTableIndex, _ uint32) bool {
			_, ok := terminatedConnectionsMap[key.Tup]
			if ok {
				return true
			}
			// Checking the flipped tuple as well.
			_, ok = terminatedConnectionsMap[netebpf.ConnTuple{
				Saddr_h:  key.Tup.Daddr_h,
				Saddr_l:  key.Tup.Daddr_l,
				Daddr_h:  key.Tup.Saddr_h,
				Daddr_l:  key.Tup.Saddr_l,
				Sport:    key.Tup.Dport,
				Dport:    key.Tup.Sport,
				Netns:    key.Tup.Netns,
				Pid:      key.Tup.Pid,
				Metadata: key.Tup.Metadata,
			}]
			return ok
		})
	dt.mapCleaner = mapCleaner
	return nil
}

// handleNewDynamicTableEntry handles a new dynamic table entry received from the eBPF programs (socket filter or
// uprobe). A new entry is added to the dynamic datastore if the value came from a Literal Header Field With Incremental
// Indexing, and to the temporary datastore if the value came from a Literal Header Field Without Indexing or Literal
// Header Field Never Indexed. If the value is Huffman encoded, it is decoded before being added to the datastores.
func (dt *DynamicTable) handleNewDynamicTableEntry(val *http2DynamicTableValue) {
	var err error
	dynamicValue := string(val.Buf[:val.String_len])
	if val.Is_huffman_encoded {
		dynamicValue, err = hpack.HuffmanDecodeToString([]byte(dynamicValue))
		if err != nil {
			log.Errorf("failed to decode huffman encoded string: %s", err)
			return
		}
	}

	ds := dt.datastore
	dsSize := dt.datastoreSize

	if val.Temporary {
		ds = dt.temporaryDatastore
		dsSize = dt.temporaryDatastoreSize
	}
	// save to dynamic
	dt.mux.Lock()
	defer dt.mux.Unlock()
	if _, ok := ds[val.Key.Tup]; !ok {
		ds[val.Key.Tup] = NewCyclicMap[uint64, string](100, func(uint64, string) {
			dsSize.Dec()
		})
	}
	// Check total length - if it's too long, we'll evict the oldest entry.
	if dsSize.Load() >= int64(dt.userModeDynamicTableSize) {
		maxCyclicMapSize := 0
		var maxCyclicMap *CyclicMap[uint64, string]
		for _, cyclicMap := range ds {
			if cyclicMap.Len() > maxCyclicMapSize {
				maxCyclicMapSize = cyclicMap.Len()
				maxCyclicMap = cyclicMap
			}
		}
		maxCyclicMap.RemoveOldest()
	}

	// Adding the new path to the dynamic table and the string.
	// This may trigger an eviction in the LRU datastore, and maybe remove the evicted entry from the kernel map.
	ds[val.Key.Tup].Add(val.Key.Index, dynamicValue)
	dsSize.Inc()
}

// launchPerfHandlerProcessor starts the perf handler used to receive new paths from the kernel.
func (dt *DynamicTable) launchPerfHandlerProcessor() error {
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
				dt.handleNewDynamicTableEntry((*http2DynamicTableValue)(unsafe.Pointer(&data.Data[0])))
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

// resolveValue resolves the value of the dynamic table for a given connection tuple and index and a temporary flag.
// If the temporary flag is set, the value will be resolved from the temporary datastore, otherwise it will be resolved
// from the datastore. The temporary datastore is used to store the values that are either Literal Header Field Without
// Indexing or Literal Header Field Never Indexed, and the datastore is used to store the values that are Literal
// Header Field With Incremental Indexing.
func (dt *DynamicTable) resolveValue(connTuple netebpf.ConnTuple, index uint64, temporary bool) (string, bool) {
	dt.mux.RLock()
	defer dt.mux.RUnlock()

	if temporary {
		cyclicMap, ok := dt.temporaryDatastore[connTuple]
		if ok {
			value, ok := cyclicMap.Pop(index)
			if ok {
				dt.temporaryDatastoreSize.Dec()
				return value, ok
			}
		}
	} else {
		cyclicMap, ok := dt.datastore[connTuple]
		if ok {
			return cyclicMap.Get(index)
		}
	}
	return "", false
}

// stop stops all the goroutines used by the dynamic table.
func (dt *DynamicTable) stop() {
	close(dt.stopChannel)
	dt.wg.Wait()

	dt.mapCleaner.Stop()

	if dt.terminatedConnectionsEventsConsumer != nil {
		dt.terminatedConnectionsEventsConsumer.Stop()
	}
}
