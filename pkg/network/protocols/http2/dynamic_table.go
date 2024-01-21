// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"go.uber.org/atomic"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	terminatedConnectionsEventStream = "terminated_http2"

	defaultMapCleanerBatchSize = 1024

	dynamicTableDefaultBufferFactor = 0.9
)

var oversizedLogLimit = util.NewLogLimit(10, time.Minute*10)

// DynamicTable encapsulates the management of the dynamic table in the user mode.
type DynamicTable struct {
	// dynamicTableSize is the size of the dynamic table
	userModeDynamicTableSize int
	// mux is used to protect the dynamic table from concurrent access
	mux sync.RWMutex
	// stopChannel is used to mark our main goroutine to stop
	stopChannel chan struct{}
	// wg is used to wait for the dynamic table to stop
	wg sync.WaitGroup
	// datastore is the datastore used to store the dynamic table entries
	datastore     map[netebpf.ConnTuple]*CyclicMap[uint64, string]
	datastoreSize atomic.Int64
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
	// We want to ensure the user-mode dynamic table is slightly smaller than the kernel one, to ensure we'll never
	// have a fully filled kernel map. When the user mode map will be full, we'll start evicting entries from the user
	// mode map, and the kernel map will be updated accordingly. Thus, keeping a small buffer in the user mode map
	// ensures we'll never fail to insert a new entry in the kernel map.
	return &DynamicTable{
		userModeDynamicTableSize: int(math.Ceil(float64(dynamicTableSize) * dynamicTableDefaultBufferFactor)),
		perfHandler:              ddebpf.NewPerfHandler(dynamicTableSize),
		stopChannel:              make(chan struct{}),
	}
}

// configureOptions configures the perf handler options for the map cleaner.
func (dt *DynamicTable) configureOptions(mgr *manager.Manager, opts *manager.Options) {
	events.Configure(terminatedConnectionsEventStream, mgr, opts)

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

	return dt.launchPerfHandlerProcessor(mgr)
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
	dynamicTableMap, _, err := mgr.GetMap(interestingDynamicTableSet)
	if err != nil {
		return fmt.Errorf("error getting %q table map: %w", interestingDynamicTableSet, err)
	}

	mapCleaner, err := ddebpf.NewMapCleaner[http2DynamicTableIndex, bool](dynamicTableMap, defaultMapCleanerBatchSize)
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
				if _, ok := dt.datastore[conn]; !ok {
					continue
				}
				dt.datastoreSize.Sub(int64(dt.datastore[conn].Len()))
				delete(dt.datastore, conn)
			}
			dt.mux.Unlock()
			terminatedConnectionsMap = make(map[netebpf.ConnTuple]struct{})
		},
		func(_ int64, key http2DynamicTableIndex, _ bool) bool {
			_, ok := terminatedConnectionsMap[key.Tup]
			return ok
		})
	dt.mapCleaner = mapCleaner
	return nil
}

// resolvePath resolves the path of a given index and connection tuple.
func (dt *DynamicTable) resolvePath(connTuple netebpf.ConnTuple, index uint64) (string, bool) {
	switch index {
	case rootPathSpecialIndex:
		return "/", true
	case indexPathSpecialIndex:
		return "/index.html", true
	}
	dt.mux.RLock()
	defer dt.mux.RUnlock()

	cyclicMap, ok := dt.datastore[connTuple]
	if !ok {
		return "", false
	}
	res, ok := cyclicMap.Get(index)
	if !ok {
		return "", false
	}
	return res, ok
}

// addPath adds a new path to the dynamic table and the string.
func (dt *DynamicTable) addPath(key http2DynamicTableIndex, path string) {
	dummyValue := true
	dt.mux.Lock()
	defer dt.mux.Unlock()
	if _, ok := dt.datastore[key.Tup]; !ok {
		currentTup := key.Tup
		dt.datastore[key.Tup] = NewCyclicMap[uint64, string](100, func(index uint64, _ string) {
			dt.datastoreSize.Dec()
			if err := dt.kernelMap.Delete(unsafe.Pointer(&http2DynamicTableIndex{
				Index: index,
				Tup:   currentTup,
			})); err != nil {
				log.Errorf("error deleting entry from the kernel map: %s; %v", err, key)
			}
		})
	}

	// Check total length - if it's too long, we'll evict the oldest entry.
	if dt.datastoreSize.Load() >= int64(dt.userModeDynamicTableSize) {
		maxCyclicMapSize := 0
		var maxCyclicMap *CyclicMap[uint64, string]
		for _, cyclicMap := range dt.datastore {
			if cyclicMap.Len() > maxCyclicMapSize {
				maxCyclicMapSize = cyclicMap.Len()
				maxCyclicMap = cyclicMap
			}
		}
		maxCyclicMap.RemoveOldest()
	}

	// Adding the new path to the dynamic table and the string.
	// This may trigger an eviction in the LRU datastore, and maybe remove the evicted entry from the kernel map.
	dt.datastore[key.Tup].Add(key.Index, path)
	dt.datastoreSize.Inc()

	// Although it is done by the kernel as well, the kernel may fail if the map is full (eviction happens in userspace),
	// thus, we do it here as well to avoid losing entries.
	// We're ignoring the error as we're trying to do best-effort here.
	if err := dt.kernelMap.Update(unsafe.Pointer(&key), unsafe.Pointer(&dummyValue), 0); err != nil {
		log.Errorf("error updating the kernel map: %s\n", err)
	}
}

// launchPerfHandlerProcessor starts the perf handler used to receive new paths from the kernel.
func (dt *DynamicTable) launchPerfHandlerProcessor(mgr *manager.Manager) error {
	kernelMap, ok, err := mgr.GetMap(interestingDynamicTableSet)
	if err != nil {
		return err
	} else if !ok {
		return errors.New("kernel map http2_interesting_dynamic_table_set not found")
	}
	dt.kernelMap = kernelMap

	dt.datastore = make(map[netebpf.ConnTuple]*CyclicMap[uint64, string], 0)

	dt.wg.Add(1)
	go func() {
		var res string
		var err error
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

				if val.Is_huffman_encoded {
					res, err = decodeHTTP2Path(val.Buf, val.String_len)
					if err != nil {
						if oversizedLogLimit.ShouldLog() {
							log.Errorf("unable to decode HTTP2 path due to: %s", err)
						}
						data.Done()
						continue
					}
				} else {
					if err = validatePathSize(val.String_len); err != nil {
						if oversizedLogLimit.ShouldLog() {
							log.Errorf("path size is invalid due to: %s", err)
						}
						data.Done()
						continue
					}

					res = string(val.Buf[:val.String_len])
					if err := validatePath(res); err != nil {
						if oversizedLogLimit.ShouldLog() {
							log.Errorf("path is invalid due to: %s", err)
						}
						data.Done()
						continue
					}
				}
				if res != "" {
					dt.addPath(val.Key, res)
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

// stop stops all the goroutines used by the dynamic table.
func (dt *DynamicTable) stop() {
	close(dt.stopChannel)
	dt.wg.Wait()

	dt.mapCleaner.Stop()

	if dt.terminatedConnectionsEventsConsumer != nil {
		dt.terminatedConnectionsEventsConsumer.Stop()
	}
}
