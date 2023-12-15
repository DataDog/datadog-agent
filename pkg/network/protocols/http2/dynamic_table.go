// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"fmt"
	"sync"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
)

const (
	terminatedConnectionsEventStream = "terminated_http2"

	defaultMapCleanerBatchSize = 1024
)

// DynamicTable encapsulates the management of the dynamic table in the user mode.
type DynamicTable struct {
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
func NewDynamicTable() *DynamicTable {
	return &DynamicTable{}
}

// configureOptions configures the perf handler options for the map cleaner.
func (dt *DynamicTable) configureOptions(mgr *manager.Manager, opts *manager.Options) {
	events.Configure(terminatedConnectionsEventStream, mgr, opts)
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

	return nil
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
			terminatedConnectionsMap = make(map[netebpf.ConnTuple]struct{})
		},
		func(_ int64, key http2DynamicTableIndex, _ bool) bool {
			_, ok := terminatedConnectionsMap[key.Tup]
			return ok
		})
	dt.mapCleaner = mapCleaner
	return nil
}

// stop stops all the goroutines used by the dynamic table.
func (dt *DynamicTable) stop() {
	dt.mapCleaner.Stop()

	if dt.terminatedConnectionsEventsConsumer != nil {
		dt.terminatedConnectionsEventsConsumer.Stop()
	}
}
