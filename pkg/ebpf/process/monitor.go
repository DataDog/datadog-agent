// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package process provides eBPF-based utilities for monitoring process events
package process

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Subscriber is an interface that allows subscribing to process start and exit events
type Subscriber interface {
	// SubscribeExec subscribes to process start events, with a callback that
	// receives the PID of the process. Returns a function that can be called to
	// unsubscribe from the event.
	SubscribeExec(func(uint32)) func()

	// SubscribeExit subscribes to process exit events, with a callback that
	// receives the PID of the process. Returns a function that can be called to
	// unsubscribe from the event.
	SubscribeExit(func(uint32)) func()

	// Sync synchronizes the contents of procfs with the internal process state,
	// calling any necessary exec/exit callbacks.
	Sync() error
}

// Monitor tracks process aliveness and calls exec/exit callbacks as appropriate.
// Sync may be called to force a synchronization with procfs state and/or
// it can happen on an interval with the SyncInterval option.
type Monitor struct {
	consumer *consumers.ProcessConsumer

	// execCallbacks holds all subscribers to process exec events
	execCallbacks *callbackMap

	// exitCallbacks holds all subscribers to process exit events
	exitCallbacks *callbackMap

	mtx       sync.RWMutex
	knownPIDs map[uint32]struct{}
	thisPID   int

	ctx          context.Context
	syncInterval time.Duration
}

// MonitorOption is an option for [Monitor]
type MonitorOption func(*Monitor)

// SyncInterval configures how often processes are synchronized with procfs.
// The context provided must be cancellable and controls when the sync loop stops.
func SyncInterval(ctx context.Context, interval time.Duration) MonitorOption {
	return func(monitor *Monitor) {
		monitor.ctx = ctx
		monitor.syncInterval = interval
	}
}

// NewMonitor creates a process monitor that tracks real-time process events.
// It also supports a configurable sync of processes from procfs to ensure no processes are missed.
func NewMonitor(consumer *consumers.ProcessConsumer, opts ...MonitorOption) (*Monitor, error) {
	thisPID, err := kernel.RootNSPID()
	if err != nil {
		return nil, fmt.Errorf("kernel root ns pid: %w", err)
	}

	m := &Monitor{
		consumer:      consumer,
		execCallbacks: newCallbackMap(),
		exitCallbacks: newCallbackMap(),
		knownPIDs:     make(map[uint32]struct{}),
		thisPID:       thisPID,
		syncInterval:  0,
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.syncInterval != 0 && m.ctx != nil {
		go m.loop(m.ctx)
	}
	return m, nil
}

// Sync synchronizes the contents of procfs with the internal process state,
// calling any necessary exec/exit callbacks.
func (m *Monitor) Sync() error {
	if !m.execCallbacks.hasCallbacks.Load() && !m.exitCallbacks.hasCallbacks.Load() {
		return nil
	}

	m.mtx.Lock()
	deletionCandidates := maps.Clone(m.knownPIDs)
	newPIDs := make(map[uint32]struct{})
	err := kernel.WithAllProcs(kernel.ProcFSRoot(), func(pid int) error {
		if pid == m.thisPID { // ignore self
			return nil
		}
		upid := uint32(pid)

		if _, ok := deletionCandidates[upid]; ok {
			// previously known process still active
			delete(deletionCandidates, upid)
			return nil
		}

		newPIDs[upid] = struct{}{}
		return nil
	})
	if err != nil {
		m.mtx.Unlock()
		return fmt.Errorf("kernel all procs: %w", err)
	}

	// modify knownPIDs while we hold mutex
	for pid := range deletionCandidates {
		delete(m.knownPIDs, pid)
	}
	for pid := range newPIDs {
		m.knownPIDs[pid] = struct{}{}
	}
	m.mtx.Unlock()

	// execute callbacks outside of holding mutex
	for pid := range deletionCandidates {
		m.exitCallbacks.call(pid)
	}
	for pid := range newPIDs {
		m.execCallbacks.call(pid)
	}
	return nil
}

func (m *Monitor) loop(ctx context.Context) {
	ticker := time.NewTicker(m.syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.Sync(); err != nil {
				log.Errorf("sync process monitor error: %v", err)
			}
		}
	}
}

func (m *Monitor) processStart(pid uint32) {
	m.mtx.RLock()
	_, known := m.knownPIDs[pid]
	m.mtx.RUnlock()
	if known {
		return
	}

	m.mtx.Lock()
	m.knownPIDs[pid] = struct{}{}
	m.mtx.Unlock()

	m.execCallbacks.call(pid)
}

func (m *Monitor) processExit(pid uint32) {
	m.mtx.RLock()
	_, known := m.knownPIDs[pid]
	m.mtx.RUnlock()
	if !known {
		return
	}

	m.mtx.Lock()
	delete(m.knownPIDs, pid)
	m.mtx.Unlock()

	m.exitCallbacks.call(pid)
}

// SubscribeExec wraps SubscribeExec from [consumers.ProcessConsumer]
func (m *Monitor) SubscribeExec(callback consumers.ProcessCallback) func() {
	m.execCallbacks.add(callback)
	return m.consumer.SubscribeExec(m.processStart)
}

// SubscribeExit wraps SubscribeExit from [consumers.ProcessConsumer]
func (m *Monitor) SubscribeExit(callback consumers.ProcessCallback) func() {
	m.exitCallbacks.add(callback)
	return m.consumer.SubscribeExit(m.processExit)
}
