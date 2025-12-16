// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package maps provides eBPF map leak detection for USM
package maps

import (
	"fmt"
	"strings"
)

// MapLeakInfo contains leak detection results for a single eBPF map
type MapLeakInfo struct {
	// MapName is the name of the eBPF map
	MapName string
	// TotalEntries is the total number of entries in the map
	TotalEntries int
	// LeakedEntries is the number of entries that appear to be leaked
	LeakedEntries int
	// LeakRate is the percentage of leaked entries (0.0 to 1.0)
	LeakRate float64
	// DeadPIDs contains the list of PIDs that no longer exist (for PID-keyed maps)
	DeadPIDs []uint32
}

// LeakDetectionReport contains the overall leak detection results
type LeakDetectionReport struct {
	// Maps contains leak information for each checked map
	Maps []MapLeakInfo
	// TotalMapsChecked is the total number of maps checked
	TotalMapsChecked int
	// MapsWithLeaks is the number of maps that have leaked entries
	MapsWithLeaks int
	// TotalLeakedEntries is the sum of all leaked entries across all maps
	TotalLeakedEntries int
	// OverallLeakRate is the overall leak rate across all checked maps
	OverallLeakRate float64
}

// String returns a human-readable summary of the leak info
func (m *MapLeakInfo) String() string {
	if m.LeakedEntries == 0 {
		return fmt.Sprintf("%s: %d/%d entries (0.0%% leaked) ✓",
			m.MapName, m.LeakedEntries, m.TotalEntries)
	}
	return fmt.Sprintf("%s: %d/%d entries (%.1f%% leaked)",
		m.MapName, m.LeakedEntries, m.TotalEntries, m.LeakRate*100)
}

// HasLeaks returns true if this map has any leaked entries
func (m *MapLeakInfo) HasLeaks() bool {
	return m.LeakedEntries > 0
}

// String returns a human-readable summary of the report
func (r *LeakDetectionReport) String() string {
	var builder strings.Builder
	builder.WriteString("USM eBPF Map Leak Detection (PID-Keyed Maps)\n")
	builder.WriteString("=============================================\n\n")

	for _, mapInfo := range r.Maps {
		builder.WriteString(mapInfo.String())
		builder.WriteString("\n")
		if mapInfo.HasLeaks() && len(mapInfo.DeadPIDs) > 0 {
			fmt.Fprintf(&builder, "  - Dead PIDs: %v\n", mapInfo.DeadPIDs)
		}
	}

	fmt.Fprintf(&builder, "\nSummary: %d leaked entries found across %d maps\n",
		r.TotalLeakedEntries, r.TotalMapsChecked)

	return builder.String()
}
