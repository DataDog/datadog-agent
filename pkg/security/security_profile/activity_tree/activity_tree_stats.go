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
	ProcessNodes    int64
	FileNodes       int64
	DNSNodes        int64
	SocketNodes     int64
	IMDSNodes       int64
	SyscallNodes    int64
	FlowNodes       int64
	CapabilityNodes int64

	// SizeBytes is an incremental estimate of the tree's heap size in bytes.
	// Updated at every insertion and decremented incrementally during eviction.
	// Periodically corrected by recomputeSizeBytes via ComputeActivityTreeStats.
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

// ApproximateSize returns the legacy shallow size estimate of the tree in bytes:
// node counts × struct header sizes. This is what V1 (legacy Manager / ActivityDump
// path) has always used to drive `activity_dump.max_dump_size` and
// `anomaly_detection.unstable_profile_size_threshold`, and the behavior is preserved
// verbatim so V1 deployments don't shift.
//
// V2 must call HeapSize() instead — it returns the incrementally-tracked real heap
// footprint (strings, slice backings, map buckets) populated by Insert/Evict paths
// and corrected by recomputeSizeBytes.
func (stats *Stats) ApproximateSize() int64 {
	var total int64
	total += stats.ProcessNodes * int64(unsafe.Sizeof(ProcessNode{}))
	total += stats.FileNodes * int64(unsafe.Sizeof(FileNode{}))
	total += stats.DNSNodes * int64(unsafe.Sizeof(DNSNode{}))
	total += stats.SocketNodes * int64(unsafe.Sizeof(SocketNode{}))
	total += stats.IMDSNodes * int64(unsafe.Sizeof(IMDSNode{}))
	total += stats.SyscallNodes * int64(unsafe.Sizeof(SyscallNode{}))
	total += stats.FlowNodes * int64(unsafe.Sizeof(FlowNode{}))
	total += stats.CapabilityNodes * int64(unsafe.Sizeof(CapabilityNode{}))
	return total
}

// HeapSize returns the tree's tracked real heap footprint in bytes (strings, slice
// backings, map buckets, struct headers). Used by V2 callers for max-size checks and
// the profile_size RAM metric.
//
// SizeBytes is maintained incrementally by Insert and Evict paths and periodically
// corrected by recomputeSizeBytes (called from ComputeActivityTreeStats). When SizeBytes
// hasn't been populated yet — proto rehydration before recompute fires, FakeOverweight,
// hand-built test fixtures — we fall back to ApproximateSize so overweight checks and
// the load controller still produce a usable number.
func (stats *Stats) HeapSize() int64 {
	if stats.SizeBytes > 0 {
		return stats.SizeBytes
	}
	return stats.ApproximateSize()
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
