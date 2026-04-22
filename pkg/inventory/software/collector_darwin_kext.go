// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// kernelExtensionsCollector collects kernel extensions (kexts) from the system
type kernelExtensionsCollector struct{}

// Collect scans kernel extension directories for installed kexts
func (c *kernelExtensionsCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning
	var itemsForPublisher []entryWithPath

	// Kernel extension directories
	// /Library/Extensions - Third-party kexts
	// /System/Library/Extensions - Apple system kexts (usually protected by SIP)
	kextDirs := []string{
		"/Library/Extensions",
	}

	for _, kextDir := range kextDirs {
		dirEntries, err := os.ReadDir(kextDir)
		if err != nil {
			// Not an error if directory doesn't exist
			if os.IsNotExist(err) {
				continue
			}
			warnings = append(warnings, warnf("failed to read directory %s: %v", kextDir, err))
			continue
		}

		for _, dirEntry := range dirEntries {
			if !strings.HasSuffix(dirEntry.Name(), ".kext") {
				continue
			}

			kextPath := filepath.Join(kextDir, dirEntry.Name())
			infoPlistPath := filepath.Join(kextPath, "Contents", "Info.plist")

			// Check if Info.plist exists
			if _, err := os.Stat(infoPlistPath); os.IsNotExist(err) {
				continue
			}

			plistData, err := readPlistFile(infoPlistPath)
			if err != nil {
				warnings = append(warnings, warnf("failed to read Info.plist for %s: %v", dirEntry.Name(), err))
				continue
			}

			// Get display name (prefer CFBundleName, fall back to bundle name)
			displayName := plistData["CFBundleName"]
			if displayName == "" {
				displayName = strings.TrimSuffix(dirEntry.Name(), ".kext")
			}

			// Get version (prefer CFBundleShortVersionString, fall back to CFBundleVersion)
			version := plistData["CFBundleShortVersionString"]
			if version == "" {
				version = plistData["CFBundleVersion"]
			}

			// Get bundle identifier as product code
			bundleID := plistData["CFBundleIdentifier"]

			// Get install date from file modification time
			var installDate string
			if info, err := os.Stat(kextPath); err == nil {
				installDate = info.ModTime().UTC().Format(time.RFC3339)
			}

			// Determine architecture
			is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

			// Check bundle integrity
			status := statusInstalled
			brokenReason := checkKextBundleIntegrity(kextPath, plistData)
			if brokenReason != "" {
				status = statusBroken
			}

			entry := &Entry{
				DisplayName:  displayName,
				Version:      version,
				InstallDate:  installDate,
				Source:       softwareTypeKext,
				ProductCode:  bundleID,
				Status:       status,
				BrokenReason: brokenReason,
				Is64Bit:      is64Bit,
				InstallPath:  kextPath,
			}

			entries = append(entries, entry)
			itemsForPublisher = append(itemsForPublisher, entryWithPath{entry: entry, path: kextPath, plistData: plistData})
		}
	}

	// Populate publisher info in parallel using code signing
	populatePublishersParallel(itemsForPublisher)

	return entries, warnings, nil
}
