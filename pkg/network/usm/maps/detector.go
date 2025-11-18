// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package maps

import (
	"errors"
	"fmt"
	"os"

	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network/usm"
)

// findMapByName searches for an eBPF map by name in the system
//
// Note: eBPF map names are truncated to 15 characters by the kernel (BPF_OBJ_NAME_LEN - 1).
// This function matches on the truncated name, which means map names MUST be unique within
// their first 15 characters to avoid collisions. For example, "hash_map_name_10" and
// "hash_map_name_11" would both truncate to "hash_map_name_1" and be indistinguishable.
//
// Currently, all USM map names satisfy this uniqueness constraint. Future work will add
// an HTTP endpoint to system-probe that can look up maps by their full names via the
// ebpf-manager, eliminating this limitation.
func findMapByName(name string) (*ebpf.Map, error) {
	var id ebpf.MapID

	// eBPF map names are limited to 15 characters (BPF_OBJ_NAME_LEN - 1)
	truncatedName := name
	if len(name) > 15 {
		truncatedName = name[:15]
	}

	for {
		var err error
		id, err = ebpf.MapGetNextID(id)
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("map %q not found", name)
		}
		if err != nil {
			return nil, fmt.Errorf("error enumerating maps: %w", err)
		}

		m, err := ebpf.NewMapFromID(id)
		if err != nil {
			continue
		}

		info, err := m.Info()
		if err != nil {
			m.Close()
			continue
		}

		// Match against the truncated name since kernel truncates to 15 chars
		if info.Name == truncatedName {
			return m, nil
		}

		m.Close()
	}
}

// checkMap finds and validates a single map, ensuring proper cleanup via defer
// Returns nil error if map is not found (allowing iteration to continue)
func (r *LeakDetectionReport) checkMap(mapName string) error {
	m, err := findMapByName(mapName)
	if err != nil {
		// Map not found - might not be loaded (e.g., TLS not enabled or system-probe not running)
		return nil
	}
	defer m.Close() // Ensure map is always closed when this function returns

	info, err := ValidatePIDKeyedMap(mapName, m)
	if err != nil {
		return fmt.Errorf("failed to validate map %s: %w", mapName, err)
	}

	r.Maps = append(r.Maps, *info)
	r.TotalMapsChecked++
	r.TotalLeakedEntries += info.LeakedEntries

	if info.HasLeaks() {
		r.MapsWithLeaks++
	}

	return nil
}

// CheckPIDKeyedMaps checks all PID-keyed TLS maps for leaked entries
// This function enumerates system-wide eBPF maps and checks each target map
func CheckPIDKeyedMaps() (*LeakDetectionReport, error) {
	pidKeyedMaps := usm.GetPIDKeyedTLSMapNames()
	report := &LeakDetectionReport{
		Maps: make([]MapLeakInfo, 0, len(pidKeyedMaps)),
	}

	for _, mapName := range pidKeyedMaps {
		if err := report.checkMap(mapName); err != nil {
			return nil, err
		}
	}

	// Calculate overall leak rate
	totalEntries := 0
	for _, mapInfo := range report.Maps {
		totalEntries += mapInfo.TotalEntries
	}

	if totalEntries > 0 {
		report.OverallLeakRate = float64(report.TotalLeakedEntries) / float64(totalEntries)
	}

	return report, nil
}
