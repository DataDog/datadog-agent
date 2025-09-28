// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	lru "github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/net/http2/hpack"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/util/intern"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	terminatedConnectionsEventStream = "terminated_http2"
	dynamicTableEventStream          = "dynamic_table"
)

// DynamicTable encapsulates the management of the dynamic table in the user mode.
type DynamicTable struct {
	cfg *config.Config

	dynamicTableEventsConsumer *events.Consumer[DynamicTableValue]
	dynamicTable               *lru.LRU[HTTP2DynamicTableIndex, *intern.StringValue]
	interner                   *intern.StringInterner

	// terminatedConnectionsEventsConsumer is the consumer used to receive terminated connections events from the kernel.
	terminatedConnectionsEventsConsumer *events.Consumer[netebpf.ConnTuple]
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
		cfg:      cfg,
		interner: intern.NewStringInterner(),
	}
}

// configureOptions configures the perf handler options for the map cleaner.
func (dt *DynamicTable) configureOptions(mgr *manager.Manager, opts *manager.Options) {
	events.Configure(dt.cfg, terminatedConnectionsEventStream, mgr, opts)
	events.Configure(dt.cfg, dynamicTableEventStream, mgr, opts)
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

	dt.dynamicTableEventsConsumer, err = events.NewConsumer(
		dynamicTableEventStream,
		mgr,
		dt.processDynamicTable,
	)

	if err != nil {
		return
	}

	dt.dynamicTable, err = lru.NewLRU[HTTP2DynamicTableIndex, *intern.StringValue](int(dt.cfg.MaxTrackedConnections), nil)
	if err != nil {
		return
	}
	dt.dynamicTableEventsConsumer.Start()
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

// processDynamicTable processes the dynamic table values sent from the kernel.
func (dt *DynamicTable) processDynamicTable(events []DynamicTableValue) {
	for _, event := range events {
		if err := dt.addDynamicTableToCache(event); err != nil {
			// TODO: Add metrics
			if oversizedLogLimit.ShouldLog() {
				log.Error(err)
			}
		}
	}
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

// sync is pulling all intermediate events from the kernel to the user mode.
func (dt *DynamicTable) sync() {
	dt.terminatedConnectionsEventsConsumer.Sync()
	dt.dynamicTableEventsConsumer.Sync()
}

// addDynamicTableToCache inserts a new value to the LRU.
func (dt *DynamicTable) addDynamicTableToCache(v DynamicTableValue) error {
	if v.Value.Is_huffman_encoded {
		if err := validatePathSize(v.Value.String_len); err != nil {
			return err
		}

		tmpBuffer := bufPool.Get().(*bytes.Buffer)
		tmpBuffer.Reset()
		defer bufPool.Put(tmpBuffer)

		n, err := hpack.HuffmanDecode(tmpBuffer, v.Value.Buffer[:v.Value.String_len])
		if err != nil {
			return err
		}

		if err := validatePath(tmpBuffer.Bytes()); err != nil {
			return err
		}

		dt.dynamicTable.Add(v.Key, dt.interner.Get(tmpBuffer.Bytes()[:n]))
		return nil
	}

	// Literal value

	if v.Value.String_len == 0 {
		return errors.New("path size: 0 is invalid")
	} else if int(v.Value.String_len) > len(v.Value.Buffer) {
		if oversizedLogLimit.ShouldLog() {
			log.Warnf("Truncating as path size: %d is greater than the buffer size: %d", v.Value.String_len, len(v.Value.Buffer))
		}
		v.Value.String_len = uint8(len(v.Value.Buffer))
	}
	if err := validatePath(v.Value.Buffer[:v.Value.String_len]); err != nil {
		return err
	}
	dt.dynamicTable.Add(v.Key, dt.interner.Get(v.Value.Buffer[:v.Value.String_len]))
	return nil
}
