// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"sync"
	"time"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

type Registerer interface {
	RegisterConnectionClosedCB(terminationManager *ConnectionTerminationManager)
}

const (
	tcpCloseEventStreamName = "tcp_close"

	// TCP connection close event maps
	tcpCloseBatchStateMap  = "tcp_close_batch_state"
	tcpCloseBatchEventsMap = "tcp_close_batch_events"
	tcpCloseBatchesMap     = "tcp_close_batches"

	// TCP connection close event flush probes
	tcpCloseNetifProbe414 = "netif_receive_skb_core_tcp_close_4_14"
	tcpCloseNetifProbe    = "tracepoint__net__netif_receive_skb_tcp_close"
)

// ConnectionCloseCallback is the function signature for connection close event handlers
// The callback receives the close event and should return quickly to avoid blocking other callbacks
type ConnectionCloseCallback func(event *EbpfConnectionCloseEvent)

// FilterFunc allows filtering events before they're dispatched to callbacks
// Return true to dispatch the event, false to skip it
type FilterFunc func(event *EbpfConnectionCloseEvent) bool

// CallbackHandle represents a registered callback that can be unregistered
type CallbackHandle struct {
	id       uint64
	callback ConnectionCloseCallback
	filter   FilterFunc
}

// ConnectionTerminationManager manages connection close event callbacks
// It receives events from the eBPF layer and dispatches them to registered callbacks
type ConnectionTerminationManager struct {
	mu             sync.RWMutex
	nextID         uint64
	callbacks      map[uint64]*CallbackHandle
	eventsConsumer *events.BatchConsumer[EbpfConnectionCloseEvent]
	mgr            *manager.Manager
	cfg            *config.Config
}

// NewConnectionTerminationManager creates a new connection termination manager
func NewConnectionTerminationManager(cfg *config.Config, mgr *manager.Manager) *ConnectionTerminationManager {
	return &ConnectionTerminationManager{
		callbacks: make(map[uint64]*CallbackHandle),
		mgr:       mgr,
		cfg:       cfg,
	}
}

// RegisterCallback registers a callback to receive connection close events
// Returns a handle that can be used to unregister the callback
// The callback will be invoked for all events that pass the optional filter
func (m *ConnectionTerminationManager) RegisterCallback(callback ConnectionCloseCallback, filter FilterFunc) *CallbackHandle {
	m.mu.Lock()
	defer m.mu.Unlock()

	handle := &CallbackHandle{
		id:       m.nextID,
		callback: callback,
		filter:   filter,
	}
	m.nextID++
	m.callbacks[handle.id] = handle

	return handle
}

// UnregisterCallback removes a previously registered callback
func (m *ConnectionTerminationManager) UnregisterCallback(handle *CallbackHandle) {
	if handle == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.callbacks, handle.id)
}

// Start initializes and starts the event consumer
func (m *ConnectionTerminationManager) Start() error {
	var err error
	m.eventsConsumer, err = events.NewBatchConsumer(
		tcpCloseEventStreamName,
		m.mgr,
		m.processConnectionCloseEvents,
	)
	if err != nil {
		return err
	}

	m.eventsConsumer.Start()

	go func() {
		for range time.Tick(10 * time.Second) {
			m.Sync()
		}
	}()
	return nil
}

// Stop stops the event consumer and cleans up resources
func (m *ConnectionTerminationManager) Stop() {
	if m.eventsConsumer != nil {
		m.eventsConsumer.Stop()
	}
}

// Sync synchronizes with the kernel by fetching all buffered events
func (m *ConnectionTerminationManager) Sync() {
	if m.eventsConsumer != nil {
		m.eventsConsumer.Sync()
	}
}

// processConnectionCloseEvents is the callback invoked by the BatchConsumer
// It dispatches events to registered callbacks
func (m *ConnectionTerminationManager) processConnectionCloseEvents(events []EbpfConnectionCloseEvent) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// If no callbacks are registered, skip processing
	if len(m.callbacks) == 0 {
		return
	}

	for i := range events {
		event := &events[i]

		// Dispatch to all registered callbacks
		for _, handle := range m.callbacks {
			// Apply filter if provided
			if handle.filter != nil && !handle.filter(event) {
				continue
			}

			// Invoke callback
			handle.callback(event)
		}
	}
}

// GetStats returns statistics about connection close events
func (m *ConnectionTerminationManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"registered_callbacks": len(m.callbacks),
	}
}

// GetMaps returns the eBPF maps required by the connection termination manager
func (m *ConnectionTerminationManager) GetMaps() []*manager.Map {
	return []*manager.Map{
		{Name: tcpCloseBatchStateMap},
		{Name: tcpCloseBatchEventsMap},
		{Name: tcpCloseBatchesMap},
	}
}

// GetProbes returns the eBPF probes required by the connection termination manager
func (m *ConnectionTerminationManager) GetProbes() []*manager.Probe {
	return []*manager.Probe{
		// {
		// 	KprobeAttachMethod: manager.AttachKprobeWithPerfEventOpen,
		// 	ProbeIdentificationPair: manager.ProbeIdentificationPair{
		// 		EBPFFuncName: tcpCloseNetifProbe414,
		// 		UID:          tcpCloseEventStreamName,
		// 	},
		// },
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tcpCloseNetifProbe,
				UID:          tcpCloseEventStreamName,
			},
		},
	}
}

// ConfigureOptions configures the manager options for the connection termination manager
// This should be called during eBPF program initialization
func (m *ConnectionTerminationManager) ConfigureOptions(opts *manager.Options) {
	// Determine which netif probe to activate based on kernel version
	tcpCloseNetifProbeID := manager.ProbeIdentificationPair{
		EBPFFuncName: tcpCloseNetifProbe,
		UID:          tcpCloseEventStreamName,
	}
	if usmconfig.ShouldUseNetifReceiveSKBCoreKprobe() {
		tcpCloseNetifProbeID.EBPFFuncName = tcpCloseNetifProbe414
	}
	opts.ActivatedProbes = append(opts.ActivatedProbes, &manager.ProbeSelector{ProbeIdentificationPair: tcpCloseNetifProbeID})

	// Configure the event stream (perf/ring buffers)
	events.Configure(m.cfg, tcpCloseEventStreamName, m.mgr, opts)

	// Enable tcp_close monitoring in eBPF
	utils.EnableOption(opts, "tcp_close_monitoring_enabled")
}

// Helper filter functions for common use cases

// FilterByProtocol returns a filter that only passes events for a specific protocol
func FilterByProtocol(protocol protocols.ProtocolType) FilterFunc {
	return func(event *EbpfConnectionCloseEvent) bool {
		s := protocols.Stack{
			API:         protocols.API(event.Stack.Api),
			Application: protocols.Application(event.Stack.Application),
			Encryption:  protocols.Encryption(event.Stack.Encryption),
		}
		return s.Contains(protocol)
	}
}

// FilterByPID returns a filter that only passes events for a specific PID
func FilterByPID(pid uint32) FilterFunc {
	return func(event *EbpfConnectionCloseEvent) bool {
		return event.Tuple.Pid == pid
	}
}

// FilterByPort returns a filter that only passes events involving a specific port
func FilterByPort(port uint16) FilterFunc {
	return func(event *EbpfConnectionCloseEvent) bool {
		return event.Tuple.Sport == port || event.Tuple.Dport == port
	}
}

// CombineFilters combines multiple filters with AND logic
func CombineFilters(filters ...FilterFunc) FilterFunc {
	return func(event *EbpfConnectionCloseEvent) bool {
		for _, filter := range filters {
			if !filter(event) {
				return false
			}
		}
		return true
	}
}
