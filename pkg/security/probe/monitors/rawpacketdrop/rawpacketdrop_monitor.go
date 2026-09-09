// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rawpacketdrop holds raw packet drop monitor related files
package rawpacketdrop

import (
	"fmt"
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// RuleIDsProvider returns the current filter index to rule_id mapping.
type RuleIDsProvider func() map[uint32]string

// Monitor reports kernel-side raw packet drop counters grouped by rule_id.
type Monitor struct {
	statsdClient statsd.ClientInterface
	droppedMap   *lib.Map
	ruleIDs      RuleIDsProvider
	lastCounts   map[string]uint64
	mu           sync.Mutex
	numCPU       int
}

// NewMonitor returns a new Monitor.
func NewMonitor(manager *manager.Manager, statsdClient statsd.ClientInterface, ruleIDs RuleIDsProvider) (*Monitor, error) {
	droppedMap, err := managerhelper.Map(manager, "dropped_packets")
	if err != nil {
		return nil, err
	}
	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}

	return &Monitor{
		statsdClient: statsdClient,
		droppedMap:   droppedMap,
		ruleIDs:      ruleIDs,
		lastCounts:   make(map[string]uint64),
		numCPU:       numCPU,
	}, nil
}

// ResetCounters clears user-space counters after the kernel map is reset.
func (m *Monitor) ResetCounters() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastCounts = make(map[string]uint64)
}

// SendStats emits deltas from the kernel dropped_packets map grouped by rule_id.
func (m *Monitor) SendStats() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// get the up to date corresponding rule IDs for each filter
	ruleIDs := m.ruleIDs()
	if len(ruleIDs) == 0 {
		m.lastCounts = make(map[string]uint64)
		return nil
	}
	currentCounts := make(map[string]uint64, len(ruleIDs))

	perCPU := make([]uint32, m.numCPU)

	var count uint64
	// get the current counts from the kernel map
	for filterIndex, ruleID := range ruleIDs {
		if ruleID == "" {
			continue
		}
		if err := m.droppedMap.Lookup(filterIndex, &perCPU); err != nil {
			seclog.Warnf("failed to lookup dropped_packets map: %s", err)
			continue
		}
		count = 0
		for _, value := range perCPU {
			count += uint64(value)
		}
		currentCounts[ruleID] += count
	}

	for ruleID, count := range currentCounts {
		last := m.lastCounts[ruleID]

		if count < last {
			seclog.Errorf("incorrect mapping leading to a negative delta for rule_id %s", ruleID)
			continue
		}
		delta := count - last
		if delta == 0 {
			continue
		}
		tags := []string{"rule_id:" + ruleID}
		if err := m.statsdClient.Count(metrics.MetricRawPacketDropped, int64(delta), tags, 1.0); err != nil {
			return fmt.Errorf("failed to send raw packet dropped metric: %w", err)
		}
		m.lastCounts[ruleID] = count
	}
	return nil
}
