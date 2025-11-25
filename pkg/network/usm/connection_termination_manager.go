// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"sync"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
)

const (
	tcpCloseEventStreamName = "tcp_close"
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

// Helper filter functions for common use cases

// FilterByProtocol returns a filter that only passes events for a specific protocol
func FilterByProtocol(protocol protocols.ProtocolType) FilterFunc {
	return func(event *EbpfConnectionCloseEvent) bool {
		// Check if the protocol is present in the stack at any layer
		app := protocols.ProtocolType(event.Stack.Application)
		enc := protocols.ProtocolType(event.Stack.Encryption)
		return app == protocol || enc == protocol
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
