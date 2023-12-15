// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Stats represents the node counts in an activity dump
type Stats struct {
	ProcessNodes int64
	FileNodes    int64
	DNSNodes     int64
	SocketNodes  int64

	processedCount map[model.EventType]*atomic.Uint64
	addedCount     map[model.EventType]map[NodeGenerationType]*atomic.Uint64
	droppedCount   map[model.EventType]map[NodeDroppedReason]*atomic.Uint64
}

// NewActivityTreeNodeStats returns a new activity tree stats
func NewActivityTreeNodeStats() *Stats {
	ats := &Stats{
		processedCount: make(map[model.EventType]*atomic.Uint64),
		addedCount:     make(map[model.EventType]map[NodeGenerationType]*atomic.Uint64),
		droppedCount:   make(map[model.EventType]map[NodeDroppedReason]*atomic.Uint64),
	}

	// generate counters
	for i := model.EventType(0); i < model.MaxKernelEventType; i++ {
		ats.processedCount[i] = atomic.NewUint64(0)
		ats.addedCount[i] = map[NodeGenerationType]*atomic.Uint64{
			Unknown:        atomic.NewUint64(0),
			Runtime:        atomic.NewUint64(0),
			Snapshot:       atomic.NewUint64(0),
			ProfileDrift:   atomic.NewUint64(0),
			WorkloadWarmup: atomic.NewUint64(0),
		}

		ats.droppedCount[i] = make(map[NodeDroppedReason]*atomic.Uint64)
		for _, reason := range allDropReasons {
			ats.droppedCount[i][reason] = atomic.NewUint64(0)
		}
	}
	return ats
}

// ApproximateSize returns an approximation of the size of the tree
func (stats *Stats) ApproximateSize() int64 {
	var total int64
	total += stats.ProcessNodes * int64(unsafe.Sizeof(ProcessNode{})) // 1024
	total += stats.FileNodes * int64(unsafe.Sizeof(FileNode{}))       // 80
	total += stats.DNSNodes * int64(unsafe.Sizeof(DNSNode{}))         // 24
	total += stats.SocketNodes * int64(unsafe.Sizeof(SocketNode{}))   // 40
	return total
}

// SendStats sends metrics to Datadog
func (stats *Stats) SendStats(client statsd.ClientInterface, treeType string) error {
	treeTypeTag := fmt.Sprintf("tree_type:%s", treeType)

	for evtType, count := range stats.processedCount {
		tags := []string{fmt.Sprintf("event_type:%s", evtType), treeTypeTag}
		if value := count.Swap(0); value > 0 {
			if err := client.Count(metrics.MetricActivityDumpEventProcessed, int64(value), tags, 1.0); err != nil {
				return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEventProcessed, err)
			}
		}
	}

	for evtType, addedCount := range stats.addedCount {
		for generationType, count := range addedCount {
			tags := []string{fmt.Sprintf("event_type:%s", evtType), fmt.Sprintf("generation_type:%s", generationType), treeTypeTag}
			if value := count.Swap(0); value > 0 {
				if err := client.Count(metrics.MetricActivityDumpEventAdded, int64(value), tags, 1.0); err != nil {
					return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEventAdded, err)
				}
			}
		}
	}

	for evtType, droppedCount := range stats.droppedCount {
		for reason, count := range droppedCount {
			tags := []string{fmt.Sprintf("event_type:%s", evtType), fmt.Sprintf("reason:%s", reason), treeTypeTag}
			if value := count.Swap(0); value > 0 {
				if err := client.Count(metrics.MetricActivityDumpEventDropped, int64(value), tags, 1.0); err != nil {
					return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEventDropped, err)
				}
			}
		}
	}

	return nil
}
