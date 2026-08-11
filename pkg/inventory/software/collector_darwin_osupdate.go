// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Plist locations, as variables so tests can point them at fixtures
var (
	systemVersionPlistPath  = "/System/Library/CoreServices/SystemVersion.plist"
	installHistoryPlistPath = "/Library/Receipts/InstallHistory.plist"
)

const (
	// macOSProductCode identifies the running operating system. It is a constant so
	// that an OS upgrade reads as a version change on one entry rather than the
	// removal of the old version and the install of a new one.
	macOSProductCode = "com.apple.macos"
	// applePublisher is the publisher reported for OS updates
	applePublisher = "Apple Inc."
	// softwareUpdateProcess is the daemon that applies OS updates; its receipts are
	// the ones worth reporting out of the full install history.
	softwareUpdateProcess = "softwareupdated"
)

// osUpdateDisplayNamePrefixes marks install-history entries as OS updates even when
// they were not applied by softwareupdated (for example, updates installed manually).
var osUpdateDisplayNamePrefixes = []string{"macOS ", "Security Update", "Security Response"}

// nonAlphanumericPattern matches runs of characters that are not slug-safe
var nonAlphanumericPattern = regexp.MustCompile(`[^a-z0-9]+`)

// osUpdateCollector collects macOS operating system updates
type osUpdateCollector struct{}

// slugifyDisplayName turns an update's display name into a stable identifier,
// e.g. "Security Update 2025-004" becomes "security-update-2025-004".
func slugifyDisplayName(name string) string {
	return strings.Trim(nonAlphanumericPattern.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// isOSUpdateReceipt reports whether an install-history entry describes an OS update
// rather than an ordinary application install.
func isOSUpdateReceipt(processName, displayName string) bool {
	if processName == softwareUpdateProcess {
		return true
	}

	for _, prefix := range osUpdateDisplayNamePrefixes {
		if strings.HasPrefix(displayName, prefix) {
			return true
		}
	}

	return false
}

// Collect returns the running system version plus any OS updates recorded in the
// install history. Failing to read the system version is fatal: macOS always runs
// some version, so an empty result would look like the OS had been uninstalled.
func (c *osUpdateCollector) Collect() ([]*Entry, []*Warning, error) {
	systemVersion, err := readPlistFile(systemVersionPlistPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read %s: %w", systemVersionPlistPath, err)
	}

	productVersion := strings.TrimSpace(systemVersion["ProductVersion"])
	if productVersion == "" {
		return nil, nil, fmt.Errorf("no ProductVersion in %s", systemVersionPlistPath)
	}

	productName := strings.TrimSpace(systemVersion["ProductName"])
	if productName == "" {
		productName = "macOS"
	}

	entries := []*Entry{{
		Source:      softwareTypeOSUpdate,
		DisplayName: productName + " " + productVersion,
		Version:     productVersion,
		Publisher:   applePublisher,
		Status:      statusInstalled,
		ProductCode: macOSProductCode,
		Is64Bit:     true,
	}}

	historyEntries, warnings := collectInstallHistory()

	return append(entries, historyEntries...), warnings, nil
}

// collectInstallHistory reads the OS updates recorded in InstallHistory.plist.
// Problems here are warnings only: the running-system entry already carries the family.
func collectInstallHistory() ([]*Entry, []*Warning) {
	var warnings []*Warning

	receipts, err := readPlistArrayFile(installHistoryPlistPath)
	if err != nil {
		return nil, []*Warning{warnf("failed to read %s: %v", installHistoryPlistPath, err)}
	}

	// The same update can be recorded more than once; keep the most recent receipt.
	newest := make(map[string]*Entry)
	installedAt := make(map[string]time.Time)

	for _, receipt := range receipts {
		displayName := strings.TrimSpace(receipt["displayName"])
		if displayName == "" {
			warnings = append(warnings, warnf("skipping install history entry with no display name"))
			continue
		}

		if !isOSUpdateReceipt(strings.TrimSpace(receipt["processName"]), displayName) {
			continue
		}

		productCode := slugifyDisplayName(displayName)
		if productCode == "" {
			warnings = append(warnings, warnf("skipping install history entry %q: no usable identifier", displayName))
			continue
		}

		var installDate string
		var installTime time.Time
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(receipt["date"])); err == nil {
			installTime = parsed
			installDate = parsed.UTC().Format(time.RFC3339)
		}

		if _, ok := newest[productCode]; ok && !installTime.After(installedAt[productCode]) {
			continue
		}

		newest[productCode] = &Entry{
			Source:      softwareTypeOSUpdate,
			DisplayName: displayName,
			Version:     strings.TrimSpace(receipt["displayVersion"]),
			InstallDate: installDate,
			Publisher:   applePublisher,
			Status:      statusInstalled,
			ProductCode: productCode,
			Is64Bit:     true,
		}
		installedAt[productCode] = installTime
	}

	entries := make([]*Entry, 0, len(newest))
	for _, entry := range newest {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ProductCode < entries[j].ProductCode })

	return entries, warnings
}
