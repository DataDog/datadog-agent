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
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	terminatedConnectionsEventStream = "terminated_http2"
)

// DynamicTable encapsulates the management of the dynamic table in the user mode.
type DynamicTable struct {
	cfg *config.Config

	// consumer is the consumer used to receive terminated connections events from the kernel.
	consumer *events.KernelAdaptiveConsumer[netebpf.ConnTuple]
	// useDirectConsumer indicates whether the direct consumer is used for the terminated connections stream.
	useDirectConsumer bool
	// terminatedConnections is the list of terminated connections received from the kernel.
	terminatedConnections []netebpf.ConnTuple
	// terminatedConnectionMux is used to protect the terminated connections list from concurrent access.
	terminatedConnectionMux sync.Mutex
	// mapCleaner is the map cleaner used to clear entries of terminated connections from the kernel map.
	mapCleaner *ddebpf.MapCleaner[HTTP2DynamicTableIndex, HTTP2DynamicTableEntry]
}

// NewDynamicTable creates a new dynamic table.
func NewDynamicTable(cfg *config.Config) (*DynamicTable, error) {
	dt := &DynamicTable{
		cfg: cfg,
	}

	// Create adaptive consumer that determines kernel version and callback internally
	if err := dt.createAdaptiveConsumer(); err != nil {
		return nil, err
	}

	return dt, nil
}

// createAdaptiveConsumer creates the appropriate consumer (direct or batch) for the
// terminated connections stream, based on configuration and kernel version.
func (dt *DynamicTable) createAdaptiveConsumer() error {
	if dt.cfg.HTTP2UseDirectConsumer {
		if events.SupportsDirectConsumer() {
			directConsumer, err := events.NewDirectConsumer(terminatedConnectionsEventStream, dt.processTerminatedConnectionsDirect, dt.cfg)
			if err != nil {
				return err
			}
			dt.consumer = events.NewKernelAdaptiveConsumer[netebpf.ConnTuple](
				directConsumer,
				[]ddebpf.Modifier{&directConsumer.EventHandler},
			)
			dt.useDirectConsumer = true
			log.Debugf("HTTP2 terminated connections monitoring: using direct consumer (requested via configuration)")
		} else {
			kernelVersion, err := kernel.HostVersion()
			if err != nil {
				log.Warnf("HTTP2 terminated connections monitoring: direct consumer requested but unable to determine kernel version (%v), falling back to batch consumer", err)
			} else {
				log.Warnf("HTTP2 terminated connections monitoring: direct consumer requested but kernel version %v < 5.8.0, falling back to batch consumer", kernelVersion)
			}
			dt.useDirectConsumer = false
		}
	} else {
		dt.useDirectConsumer = false
	}

	return nil
}

// Modifiers returns the eBPF manager modifiers required by the terminated connections consumer.
func (dt *DynamicTable) Modifiers() []ddebpf.Modifier {
	if dt.consumer == nil {
		return nil
	}
	return dt.consumer.Modifiers()
}

// configureOptions configures the perf handler options for the map cleaner.
func (dt *DynamicTable) configureOptions(mgr *manager.Manager, opts *manager.Options) {
	events.Configure(dt.cfg, terminatedConnectionsEventStream, mgr, opts)
}

// preStart sets up the terminated connections events consumer.
func (dt *DynamicTable) preStart(mgr *manager.Manager) error {
	// If using BatchConsumer, create it now (after manager initialization).
	// The DirectConsumer is already created in NewDynamicTable via createAdaptiveConsumer().
	if !dt.useDirectConsumer {
		batchConsumer, err := events.NewBatchConsumer(terminatedConnectionsEventStream, mgr, dt.processTerminatedConnections)
		if err != nil {
			return err
		}
		dt.consumer = events.NewKernelAdaptiveConsumer[netebpf.ConnTuple](
			batchConsumer,
			[]ddebpf.Modifier{}, // BatchConsumer needs no modifiers
		)
	}

	dt.consumer.Start()

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

// processTerminatedConnectionsDirect processes a single terminated connection received
// from the kernel via the direct consumer.
func (dt *DynamicTable) processTerminatedConnectionsDirect(event *netebpf.ConnTuple) {
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
			dt.consumer.Sync()
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

	if dt.consumer != nil {
		dt.consumer.Stop()
	}
}
