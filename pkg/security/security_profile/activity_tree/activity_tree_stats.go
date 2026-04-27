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

	// FileNodesMerged counts siblings absorbed by path-pattern merges.
	// Incremented once per node folded into an existing template.
	FileNodesMerged int64
	// FilePatternsCreated counts merge operations that collapsed a
	// signature bucket into a new template FileNode.
	FilePatternsCreated int64
	// FilePatternLookupHits counts insertions that matched an existing
	// pattern node via the wildcard fallback — i.e. insertions that
	// would have created a new FileNode without path-pattern merging.
	FilePatternLookupHits int64

	// patternCfg carries the per-tree path-pattern mining configuration.
	// It is the single source of truth for whether mining runs on this
	// tree — v2 callers flip Enabled via SetPathPatternConfig; v1 and
	// activity dumps leave it zero-valued (disabled).
	//
	// Lives on Stats rather than ActivityTree because Stats is already
	// threaded through every insertion site (maybeMergeChildren,
	// findChildWithPatternFallback) and we want the gate reachable
	// without changing those signatures. The field is unexported and
	// therefore not serialized by the protobuf / JSON encoders.
	patternCfg PathPatternConfig

	counts map[model.EventType]*statsPerEventType
}

// SetPathPatternConfig overrides the path-pattern mining configuration
// for the tree owning these stats. Callers that want mining enabled
// should pass DefaultPathPatternConfig(); the zero-value configuration
// leaves mining disabled and every gate short-circuits.
func (stats *Stats) SetPathPatternConfig(cfg PathPatternConfig) {
	if stats == nil {
		return
	}
	stats.patternCfg = cfg
}

// PathPatternConfig returns the currently configured mining parameters
// (useful for tests and introspection).
func (stats *Stats) PathPatternConfig() PathPatternConfig {
	if stats == nil {
		return PathPatternConfig{}
	}
	return stats.patternCfg
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

// ApproximateSize returns an approximation of the size of the tree
func (stats *Stats) ApproximateSize() int64 {
	var total int64
	total += stats.ProcessNodes * int64(unsafe.Sizeof(ProcessNode{})) // 1024
	total += stats.FileNodes * int64(unsafe.Sizeof(FileNode{}))       // 80
	total += stats.DNSNodes * int64(unsafe.Sizeof(DNSNode{}))         // 24
	total += stats.SocketNodes * int64(unsafe.Sizeof(SocketNode{}))   // 40
	total += stats.IMDSNodes * int64(unsafe.Sizeof(IMDSNode{}))
	total += stats.SyscallNodes * int64(unsafe.Sizeof(SyscallNode{}))
	total += stats.FlowNodes * int64(unsafe.Sizeof(FlowNode{}))
	total += stats.CapabilityNodes * int64(unsafe.Sizeof(CapabilityNode{}))
	return total
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

	// Path-pattern mining counters. These are cumulative over the tree's
	// lifetime and are reset after each send so they behave as rates.
	// The same single-goroutine-insertion assumption as the rest of Stats
	// applies — no atomics are used for consistency with FileNodes.
	treeTag := []string{treeTypeTag}
	if value := stats.FileNodesMerged; value > 0 {
		stats.FileNodesMerged = 0
		if err := client.Count(metrics.MetricActivityDumpFileNodesMerged, value, treeTag, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpFileNodesMerged, err)
		}
	}
	if value := stats.FilePatternsCreated; value > 0 {
		stats.FilePatternsCreated = 0
		if err := client.Count(metrics.MetricActivityDumpFilePatternsCreated, value, treeTag, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpFilePatternsCreated, err)
		}
	}
	if value := stats.FilePatternLookupHits; value > 0 {
		stats.FilePatternLookupHits = 0
		if err := client.Count(metrics.MetricActivityDumpFilePatternLookupHits, value, treeTag, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpFilePatternLookupHits, err)
		}
	}

	return nil
}
