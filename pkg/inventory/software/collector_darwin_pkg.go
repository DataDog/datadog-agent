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
)

// pkgReceiptsCollector collects software from PKG installer receipts
// This collector filters out receipts for applications that are already captured
// by the applicationsCollector (apps in /Applications), to avoid confusing duplicates.
type pkgReceiptsCollector struct{}

// Collect reads PKG installer receipts from /var/db/receipts
// It filters out:
//   - Mac App Store receipts (ending in _MASReceipt) - these are handled by applicationsCollector
//   - Packages with InstallPrefixPath starting with "Applications" - already captured by applicationsCollector
//
// Packages with prefix "/" that also install to /Applications are handled by
// deduplication in GetSoftwareInventoryWithCollectors, which drops PKG entries
// whose ProductCode matches an applicationsCollector entry's PkgID.
func (c *pkgReceiptsCollector) Collect() ([]*Entry, []*Warning, error) {

	var entries []*Entry
	var warnings []*Warning

	receiptsDir := "/var/db/receipts"

	dirEntries, err := os.ReadDir(receiptsDir)
	if err != nil {
		// Not an error if receipts directory doesn't exist
		if os.IsNotExist(err) {
			return entries, warnings, nil
		}
		return nil, nil, err
	}

	// Determine architecture
	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	for _, dirEntry := range dirEntries {
		if !strings.HasSuffix(dirEntry.Name(), ".plist") {
			continue
		}

		receiptPath := filepath.Join(receiptsDir, dirEntry.Name())
		plistData, err := readPlistFile(receiptPath)
		if err != nil {
			warnings = append(warnings, warnf("failed to read receipt %s: %v", dirEntry.Name(), err))
			continue
		}

		// Get package identifier as both display name and product code
		packageID := plistData["PackageIdentifier"]
		if packageID == "" {
			continue
		}

		// Skip Mac App Store receipts - these correspond to MAS apps which are
		// already captured by applicationsCollector with richer metadata
		if strings.HasSuffix(packageID, "_MASReceipt") {
			continue
		}

		// Get install prefix path from receipt
		prefixPath := plistData["InstallPrefixPath"]
		if prefixPath == "" {
			prefixPath = plistData["InstallLocation"]
		}

		// Skip packages whose install prefix is under Applications — these are
		// application bundles already captured by applicationsCollector.
		if strings.HasPrefix(prefixPath, "Applications") {
			continue
		}

		// Determine install_path from the prefix
		var installPath string
		if prefixPath != "" && prefixPath != "/" {
			if !strings.HasPrefix(prefixPath, "/") {
				installPath = "/" + prefixPath
			} else {
				installPath = prefixPath
			}
		} else {
			installPath = "N/A"
		}

		// Check if the installation location still exists
		status := statusInstalled
		var brokenReason string
		if installPath != "" && installPath != "N/A" {
			if _, err := os.Stat(installPath); os.IsNotExist(err) {
				status = statusBroken
				brokenReason = "install path not found: " + installPath
			}
		}

		entry := &Entry{
			DisplayName:  packageID,
			Version:      plistData["PackageVersion"],
			InstallDate:  plistData["InstallDate"],
			Source:       softwareTypePkg,
			ProductCode:  packageID,
			Status:       status,
			BrokenReason: brokenReason,
			Is64Bit:      is64Bit,
			InstallPath:  installPath,
		}

		entries = append(entries, entry)
	}

	return entries, warnings, nil
}
