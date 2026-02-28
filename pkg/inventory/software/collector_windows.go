// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"fmt"
	"strings"
	"time"
)

// defaultCollectors returns the default collectors for production use
func defaultCollectors() []Collector {
	return []Collector{
		// desktopAppCollector aggregates MSI and Registry collectors
		&desktopAppCollector{},
		// msStoreAppsCollector collects Windows Store apps
		&msStoreAppsCollector{},
	}
}

// desktopAppCollector aggregates multiple sources to identify desktop apps.
// It will flag apps that are in broken states by comparing them between multiple sources.
// I.e. if an application is present in the MSI database and not in the registry.
type desktopAppCollector struct{}

func convertTimestamp(dateStr string) (string, error) {
	var t time.Time
	var err error

	t, err = time.Parse("2006-01-02", dateStr)
	if err != nil {
		t, err = time.Parse("20060102", dateStr)
		if err != nil {
			return "", fmt.Errorf("unable to parse date: %v", err)
		}
	}

	// Convert to RFC3339Nano format
	return t.UTC().Format(time.RFC3339Nano), nil
}

func (d *desktopAppCollector) Collect() ([]*Entry, []*Warning, error) {
	regCollector := registryCollector{}
	regEntries, regWarnings, err := regCollector.Collect()
	if err != nil {
		return nil, regWarnings, err
	}
	// Build a map of software entry for quick lookup
	regMap := map[string]*Entry{}
	for _, regEntry := range regEntries {
		regMap[regEntry.GetID()] = regEntry
	}

	msiCollector := mSICollector{}
	msiEntries, msiWarnings, err := msiCollector.Collect()
	if err != nil {
		return nil, msiWarnings, err
	}

	for _, msiEntry := range msiEntries {
		if regEntry, ok := regMap[msiEntry.GetID()]; !ok {
			// Software is present in MSI but not in registry
			msiEntry.Status = "broken"
			msiEntry.BrokenReason = "MSI record not found in registry"
			regEntries = append(regEntries, msiEntry)
		} else {
			if regEntry.InstallDate == "" {
				regEntry.InstallDate = msiEntry.InstallDate
			}
		}
	}

	return regEntries, append(regWarnings, msiWarnings...), nil
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
