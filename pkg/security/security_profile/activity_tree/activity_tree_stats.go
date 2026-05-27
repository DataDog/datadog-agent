// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"fmt"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Stats represents the node counts in an activity dump
type Stats struct {
	ProcessNodes    int64
	FileNodes       int64
	DNSNodes        int64
	SocketNodes     int64
	IMDSNodes       int64
	SyscallNodes    int64
	FlowNodes       int64
	CapabilityNodes int64

	// SizeBytes is an incremental estimate of the tree's heap size in bytes.
	// It is updated at every insertion and recomputed from scratch after evictions.
	SizeBytes int64

	counts map[model.EventType]*statsPerEventType
}

type statsPerEventType struct {
	processedCount *atomic.Uint64
	addedCount     map[NodeGenerationType]*atomic.Uint64
	droppedCount   map[NodeDroppedReason]*atomic.Uint64
}

// NewActivityTreeNodeStats returns a new activity tree stats
func NewActivityTreeNodeStats() *Stats {
	ats := &Stats{
		counts: make(map[model.EventType]*statsPerEventType),
	}

	// generate counters
	for i := model.EventType(0); i < model.MaxKernelEventType; i++ {
		spet := &statsPerEventType{
			processedCount: atomic.NewUint64(0),
			addedCount: map[NodeGenerationType]*atomic.Uint64{
				Unknown:        atomic.NewUint64(0),
				Runtime:        atomic.NewUint64(0),
				Snapshot:       atomic.NewUint64(0),
				ProfileDrift:   atomic.NewUint64(0),
				WorkloadWarmup: atomic.NewUint64(0),
			},
			droppedCount: make(map[NodeDroppedReason]*atomic.Uint64),
		}

		for reason := minNodeDroppedReason; reason <= maxNodeDroppedReason; reason++ {
			spet.droppedCount[reason] = atomic.NewUint64(0)
		}
		ats.counts[i] = spet
	}
	return ats
}

// ApproximateSize returns the tracked in-memory size of the tree in bytes.
// This value is updated incrementally at insertion time and recomputed after evictions.
func (stats *Stats) ApproximateSize() int64 {
	return stats.SizeBytes
}

// SendStats sends metrics to Datadog
func (stats *Stats) SendStats(client statsd.ClientInterface, treeType string) error {
	treeTypeTag := "tree_type:" + treeType

	tags := []string{treeTypeTag, "", ""}
	for evtType, count := range stats.counts {
		evtTypeTag := "event_type:" + evtType.String()

		tags[1] = evtTypeTag
		if value := count.processedCount.Swap(0); value > 0 {
			if err := client.Count(metrics.MetricActivityDumpEventProcessed, int64(value), tags[:1], 1.0); err != nil {
				return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEventProcessed, err)
			}
		}

		for generationType, count := range count.addedCount {
			tags[2] = generationType.Tag()
			if value := count.Swap(0); value > 0 {
				if err := client.Count(metrics.MetricActivityDumpEventAdded, int64(value), tags, 1.0); err != nil {
					return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEventAdded, err)
				}
			}
		}

		for reason, count := range count.droppedCount {
			tags[2] = reason.Tag()
			if value := count.Swap(0); value > 0 {
				if err := client.Count(metrics.MetricActivityDumpEventDropped, int64(value), tags, 1.0); err != nil {
					return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEventDropped, err)
				}
			}
		}
	}

	return nil
}
