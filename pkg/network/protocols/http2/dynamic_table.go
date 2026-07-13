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
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
)

const (
	terminatedConnectionsEventStream = "terminated_http2"
)

// DynamicTable encapsulates the management of the dynamic table in the user mode.
type DynamicTable struct {
	cfg *config.Config

	// terminatedConnectionsEventsConsumer is the consumer used to receive terminated connections events from the kernel.
	// It wraps either a batch or a direct consumer, matching the mode selected for the main http2 stream.
	terminatedConnectionsEventsConsumer *events.KernelAdaptiveConsumer[netebpf.ConnTuple]
	// useDirectConsumer indicates whether the terminated connections stream uses the direct consumer.
	useDirectConsumer bool
	// terminatedConnections is the list of terminated connections received from the kernel.
	terminatedConnections []netebpf.ConnTuple
	// terminatedConnectionMux is used to protect the terminated connections list from concurrent access.
	terminatedConnectionMux sync.Mutex
	// mapCleaner is the map cleaner used to clear entries of terminated connections from the kernel map.
	mapCleaner *ddebpf.MapCleaner[HTTP2DynamicTableIndex, HTTP2DynamicTableEntry]
}

// NewDynamicTable creates a new dynamic table.
func NewDynamicTable(cfg *config.Config) *DynamicTable {
	return &DynamicTable{
		cfg: cfg,
	}
}

// createDirectConsumer sets up the terminated connections stream to use the
// direct consumer. It MUST be called with the same decision as the main http2
// stream (Protocol.useDirectConsumer): both streams share the single
// http2_use_direct_consumer eBPF constant, so the kernel output path and the
// userspace consumer type must agree, otherwise terminated events would be
// emitted on the direct path while a batch consumer reads them (or vice versa).
//
// When direct mode is not selected, this is a no-op: the batch consumer is
// created later in preStart (after manager init), since it needs no modifier.
// When it is selected, the direct consumer must be created here so its
// EventHandler modifier is exposed via modifiers() before the manager is
// initialized (right after protocol construction).
func (dt *DynamicTable) createDirectConsumer(useDirectConsumer bool) error {
	if !useDirectConsumer {
		return nil
	}

	directConsumer, err := events.NewDirectConsumer(terminatedConnectionsEventStream, dt.processTerminatedConnectionDirect, dt.cfg)
	if err != nil {
		return err
	}
	dt.terminatedConnectionsEventsConsumer = events.NewKernelAdaptiveConsumer[netebpf.ConnTuple](
		directConsumer,
		[]ddebpf.Modifier{&directConsumer.EventHandler},
	)
	dt.useDirectConsumer = true
	return nil
}

// modifiers returns the eBPF manager modifiers contributed by the terminated
// connections consumer (the direct consumer's EventHandler, if any).
func (dt *DynamicTable) modifiers() []ddebpf.Modifier {
	if dt.terminatedConnectionsEventsConsumer == nil {
		return nil
	}
	return dt.terminatedConnectionsEventsConsumer.Modifiers()
}

// configureOptions configures the perf handler options for the map cleaner.
func (dt *DynamicTable) configureOptions(mgr *manager.Manager, opts *manager.Options) {
	events.Configure(dt.cfg, terminatedConnectionsEventStream, mgr, opts)
}

// preStart sets up the terminated connections events consumer.
func (dt *DynamicTable) preStart(mgr *manager.Manager) (err error) {
	// If using the batch consumer, create it now (after manager initialization).
	// The direct consumer, when used, was already created in NewDynamicTable so
	// its modifier could be registered before the manager was initialized.
	if !dt.useDirectConsumer {
		batchConsumer, err := events.NewBatchConsumer(
			terminatedConnectionsEventStream,
			mgr,
			dt.processTerminatedConnections,
		)
		if err != nil {
			return err
		}
		dt.terminatedConnectionsEventsConsumer = events.NewKernelAdaptiveConsumer[netebpf.ConnTuple](
			batchConsumer,
			[]ddebpf.Modifier{}, // BatchConsumer needs no modifiers
		)
	}

	// Start the consumer (works for both direct and batch consumers).
	dt.terminatedConnectionsEventsConsumer.Start()

	return nil
}

// postStart sets up the dynamic table map cleaner.
func (dt *DynamicTable) postStart(mgr *manager.Manager, cfg *config.Config) error {
	return dt.setupDynamicTableMapCleaner(mgr, cfg)
}

// processTerminatedConnections processes a batch of terminated connections received from the kernel.
func (dt *DynamicTable) processTerminatedConnections(events []netebpf.ConnTuple) {
	dt.terminatedConnectionMux.Lock()
	defer dt.terminatedConnectionMux.Unlock()
	dt.terminatedConnections = append(dt.terminatedConnections, events...)
}

// processTerminatedConnectionDirect processes a single terminated connection received via the direct consumer.
func (dt *DynamicTable) processTerminatedConnectionDirect(event *netebpf.ConnTuple) {
	dt.terminatedConnectionMux.Lock()
	defer dt.terminatedConnectionMux.Unlock()
	dt.terminatedConnections = append(dt.terminatedConnections, *event)
}

// setupDynamicTableMapCleaner sets up the map cleaner used to clear entries of terminated connections from the kernel map.
func (dt *DynamicTable) setupDynamicTableMapCleaner(mgr *manager.Manager, cfg *config.Config) error {
	dynamicTableMap, _, err := mgr.GetMap(dynamicTable)
	if err != nil {
		return fmt.Errorf("error getting http2 dynamic table map: %w", err)
	}

	mapCleaner, err := ddebpf.NewMapCleaner[HTTP2DynamicTableIndex, HTTP2DynamicTableEntry](dynamicTableMap, protocols.DefaultMapCleanerBatchSize, dynamicTable, "usm_monitor")
	if err != nil {
		return fmt.Errorf("error creating a map cleaner for http2 dynamic table: %w", err)
	}

	terminatedConnectionsMap := make(map[netebpf.ConnTuple]struct{})
	mapCleaner.Start(cfg.HTTP2DynamicTableMapCleanerInterval,
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
		func(_ int64, key HTTP2DynamicTableIndex, _ HTTP2DynamicTableEntry) bool {
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

// stop stops all the goroutines used by the dynamic table.
func (dt *DynamicTable) stop() {
	dt.mapCleaner.Stop()

	if dt.terminatedConnectionsEventsConsumer != nil {
		dt.terminatedConnectionsEventsConsumer.Stop()
	}
}
