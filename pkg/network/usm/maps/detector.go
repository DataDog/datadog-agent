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
)

// PIDKeyedTLSMaps contains the names of all TLS eBPF maps that use pid_tgid as keys
var PIDKeyedTLSMaps = []string{
	"ssl_read_args",
	"ssl_read_ex_args",
	"ssl_write_args",
	"ssl_write_ex_args",
	"bio_new_socket_args",
	"ssl_ctx_by_pid_tgid",
}

// findMapByName searches for an eBPF map by name in the system
// Note: eBPF map names are truncated to 15 characters in the kernel,
// so we match on the truncated name for longer map names
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

// CheckPIDKeyedMaps checks all PID-keyed TLS maps for leaked entries
// This function enumerates system-wide eBPF maps and checks each target map
func CheckPIDKeyedMaps() (*LeakDetectionReport, error) {
	report := &LeakDetectionReport{
		Maps: make([]MapLeakInfo, 0, len(PIDKeyedTLSMaps)),
	}

	for _, mapName := range PIDKeyedTLSMaps {
		m, err := findMapByName(mapName)
		if err != nil {
			// Map not found - might not be loaded (e.g., TLS not enabled or system-probe not running)
			continue
		}

		info, err := ValidatePIDKeyedMap(mapName, m)
		m.Close() // Close immediately after validation

		if err != nil {
			return nil, fmt.Errorf("failed to validate map %s: %w", mapName, err)
		}

		report.Maps = append(report.Maps, *info)
		report.TotalMapsChecked++
		report.TotalLeakedEntries += info.LeakedEntries

		if info.HasLeaks() {
			report.MapsWithLeaks++
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
