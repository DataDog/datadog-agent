// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package winsoftware implements code to collect installed software from a Windows system.
package winsoftware

import (
	"strings"
)

// SoftwareCollector defines the interface for collecting software entries
type SoftwareCollector interface {
	// Collect returns a list of software entries and any warnings encountered
	Collect() ([]*SoftwareEntry, []*Warning, error)
}

// defaultCollectors returns the default collectors for production use
func defaultCollectors() []SoftwareCollector {
	return []SoftwareCollector{
		&MSICollector{},
		&RegistryCollector{},
	}
}

// GetSoftwareInventory returns a list of software entries found on the system
func GetSoftwareInventory() ([]*SoftwareEntry, []*Warning, error) {
	return GetSoftwareInventoryWithCollectors(defaultCollectors())
}

// GetSoftwareInventoryWithCollectors returns a list of software entries using the provided collectors
func GetSoftwareInventoryWithCollectors(collectors []SoftwareCollector) ([]*SoftwareEntry, []*Warning, error) {
	var warn []*Warning
	var allEntries []*SoftwareEntry

	// Collect from all sources
	for _, collector := range collectors {
		entries, warnings, err := collector.Collect()
		if err != nil {
			// Log error but continue with other collectors
			warn = append(warn, warnf("error collecting software: %v", err))
			continue
		}

		// Add any warnings from the collector
		warn = append(warn, warnings...)

		// Add entries to result list
		for _, entry := range entries {
			if entry == nil {
				warn = append(warn, warnf("invalid software detected"))
				continue
			}
			allEntries = append(allEntries, entry)
		}
	}

	return allEntries, warn, nil
}

// trimVersion trims leading zeros from each part of a version string.
// For example, "4.08.09032" becomes "4.8.9032".
// That makes it easier to compare versions as strings, as Windows will sometimes trim leading zeros.
func trimVersion(version string) string {
	parts := strings.Split(version, ".")
	for i, part := range parts {
		trimmed := strings.TrimLeft(part, "0")
		if trimmed == "" {
			trimmed = "0"
		}
		parts[i] = trimmed
	}
	return strings.Join(parts, ".")
}
