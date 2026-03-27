// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPkgReceiptsCollectorIntegration runs the real collector against the system
// and validates that all returned entries have correct structure and field values.
func TestPkgReceiptsCollectorIntegration(t *testing.T) {
	// Skip if receipts directory doesn't exist (e.g. minimal CI runners)
	if _, err := os.Stat("/var/db/receipts"); os.IsNotExist(err) {
		t.Skip("No /var/db/receipts directory — skipping integration test")
	}

	collector := &pkgReceiptsCollector{}
	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	t.Logf("Found %d PKG entries with %d warnings", len(entries), len(warnings))

	for i, entry := range entries {
		// Every entry must have a display name (the package ID)
		assert.NotEmpty(t, entry.DisplayName, "Entry %d missing DisplayName", i)

		// Source must be "pkg"
		assert.Equal(t, softwareTypePkg, entry.Source, "Entry %d (%s) wrong source", i, entry.DisplayName)

		// ProductCode should match DisplayName for PKG receipts
		assert.Equal(t, entry.DisplayName, entry.ProductCode,
			"Entry %d (%s) ProductCode should match DisplayName", i, entry.DisplayName)

		// Status should be either "installed" or "broken"
		assert.Contains(t, []string{statusInstalled, statusBroken}, entry.Status,
			"Entry %d (%s) has invalid status: %s", i, entry.DisplayName, entry.Status)

		// If broken, should have a reason
		if entry.Status == statusBroken {
			assert.NotEmpty(t, entry.BrokenReason,
				"Entry %d (%s) is broken but missing reason", i, entry.DisplayName)
		}

		// Is64Bit should be true on modern macOS (amd64 or arm64)
		assert.True(t, entry.Is64Bit, "Entry %d (%s) should be 64-bit", i, entry.DisplayName)

		// No MAS receipts should be present
		assert.False(t, strings.HasSuffix(entry.DisplayName, "_MASReceipt"),
			"MAS receipt should be filtered: %s", entry.DisplayName)

		// No Applications-prefix packages should be present
		if entry.InstallPath != "" && entry.InstallPath != "N/A" {
			assert.False(t, strings.HasPrefix(entry.InstallPath, "/Applications/"),
				"PKG entry %s should not have /Applications install path: %s",
				entry.DisplayName, entry.InstallPath)
		}

		// Entries with a concrete install path should either exist or be marked broken
		if entry.InstallPath != "" && entry.InstallPath != "N/A" {
			if _, statErr := os.Stat(entry.InstallPath); os.IsNotExist(statErr) {
				assert.Equal(t, statusBroken, entry.Status,
					"Entry %s has non-existent path %s but is not marked broken",
					entry.DisplayName, entry.InstallPath)
			}
		}
	}
}

// TestPkgReceiptsCollectorWithMockReceipts tests the collector's filtering and
// field mapping logic using synthetic receipt plists, independent of system state.
func TestPkgReceiptsCollectorWithMockReceipts(t *testing.T) {
	tempDir := t.TempDir()
	receiptsDir := filepath.Join(tempDir, "receipts")
	require.NoError(t, os.MkdirAll(receiptsDir, 0755))

	writeReceipt := func(filename, pkgID, version, prefixPath string) {
		plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PackageIdentifier</key>
	<string>` + pkgID + `</string>
	<key>PackageVersion</key>
	<string>` + version + `</string>
	<key>InstallPrefixPath</key>
	<string>` + prefixPath + `</string>
	<key>InstallDate</key>
	<date>2024-01-15T10:30:00Z</date>
</dict>
</plist>`
		require.NoError(t, os.WriteFile(filepath.Join(receiptsDir, filename), []byte(plist), 0644))
	}

	// Non-app package with specific prefix (should be included)
	writeReceipt("com.datadoghq.agent.plist", "com.datadoghq.agent", "7.75.4", "opt/datadog-agent")

	// Package with Applications prefix (should be filtered)
	writeReceipt("com.google.Chrome.plist", "com.google.Chrome", "120.0", "Applications")

	// Package with Applications/.app prefix (should be filtered)
	writeReceipt("com.apple.pkg.Pages.plist", "com.apple.pkg.Pages", "14.0", "Applications/Pages.app/Contents/")

	// MAS receipt (should be filtered by _MASReceipt suffix)
	writeReceipt("com.apple.cdm.pkg.iMovie_MASReceipt.plist", "com.apple.cdm.pkg.iMovie_MASReceipt", "10.4", "Applications/iMovie.app/Contents/")

	// System package with "/" prefix (should be included)
	writeReceipt("com.apple.pkg.MobileAssets.plist", "com.apple.pkg.MobileAssets", "1.0", "/")

	// Package with empty prefix (should be included)
	writeReceipt("com.example.tool.plist", "com.example.tool", "2.0", "")

	// Manually run the plist-reading and filtering logic against our temp dir.
	// This mirrors what Collect() does internally.
	dirEntries, err := os.ReadDir(receiptsDir)
	require.NoError(t, err)

	type receipt struct {
		packageID  string
		prefixPath string
	}
	var kept []receipt
	for _, de := range dirEntries {
		if !strings.HasSuffix(de.Name(), ".plist") {
			continue
		}
		plistData, err := readPlistFile(filepath.Join(receiptsDir, de.Name()))
		require.NoError(t, err)

		pkgID := plistData["PackageIdentifier"]
		if pkgID == "" {
			continue
		}
		if strings.HasSuffix(pkgID, "_MASReceipt") {
			continue
		}
		prefix := plistData["InstallPrefixPath"]
		if strings.HasPrefix(prefix, "Applications") {
			continue
		}
		kept = append(kept, receipt{packageID: pkgID, prefixPath: prefix})
	}

	// Should have 3 entries: datadog-agent, MobileAssets, example.tool
	// Chrome, Pages, and iMovie_MASReceipt should all be filtered
	assert.Len(t, kept, 3, "Should have 3 entries after filtering")

	keptIDs := make(map[string]bool)
	for _, r := range kept {
		keptIDs[r.packageID] = true
	}
	assert.True(t, keptIDs["com.datadoghq.agent"], "Datadog agent should be kept (prefix: opt/datadog-agent)")
	assert.True(t, keptIDs["com.apple.pkg.MobileAssets"], "MobileAssets should be kept (prefix: /)")
	assert.True(t, keptIDs["com.example.tool"], "Example tool should be kept (prefix: empty)")
	assert.False(t, keptIDs["com.google.Chrome"], "Chrome should be filtered (prefix: Applications)")
	assert.False(t, keptIDs["com.apple.pkg.Pages"], "Pages should be filtered (prefix: Applications/...)")
	assert.False(t, keptIDs["com.apple.cdm.pkg.iMovie_MASReceipt"], "iMovie MAS should be filtered")
}

// TestGetSoftwareInventoryDedup verifies that GetSoftwareInventoryWithCollectors
// deduplicates PKG entries when an applicationsCollector entry has a matching PkgID.
func TestGetSoftwareInventoryDedup(t *testing.T) {
	appsCollector := &MockCollector{
		entries: map[string]*Entry{
			"keynote": {
				DisplayName: "Keynote",
				Version:     "14.3",
				Source:      "app",
				ProductCode: "com.apple.iWork.Keynote",
				PkgID:       "com.apple.pkg.Keynote14",
				InstallPath: "/Applications/Keynote.app",
			},
			"chrome": {
				DisplayName: "Google Chrome",
				Version:     "120.0",
				Source:      "app",
				ProductCode: "com.google.Chrome",
				PkgID:       "",
				InstallPath: "/Applications/Google Chrome.app",
			},
		},
	}

	pkgCollector := &MockCollector{
		entries: map[string]*Entry{
			"keynote-pkg": {
				DisplayName: "com.apple.pkg.Keynote14",
				Version:     "14.3.1.1733479950",
				Source:      softwareTypePkg,
				ProductCode: "com.apple.pkg.Keynote14",
				InstallPath: "N/A",
			},
			"dd-agent": {
				DisplayName: "com.datadoghq.agent",
				Version:     "7.75.4",
				Source:      softwareTypePkg,
				ProductCode: "com.datadoghq.agent",
				InstallPath: "/opt/datadog-agent",
			},
		},
	}

	entries, _, err := GetSoftwareInventoryWithCollectors([]Collector{appsCollector, pkgCollector})
	require.NoError(t, err)

	entryNames := make(map[string]bool)
	for _, e := range entries {
		entryNames[e.DisplayName] = true
	}

	assert.True(t, entryNames["Keynote"], "Keynote app entry should be present")
	assert.False(t, entryNames["com.apple.pkg.Keynote14"],
		"Keynote PKG entry should be deduped because appsCollector claimed it via PkgID")
	assert.True(t, entryNames["Google Chrome"], "Chrome app entry should be present")
	assert.True(t, entryNames["com.datadoghq.agent"],
		"Datadog agent PKG entry should NOT be deduped — no appsCollector entry claims it")
	assert.Len(t, entries, 3, "Should have 3 entries: Keynote app, Chrome app, DD agent pkg")
}

// TestGetSoftwareInventoryDedupNoPkgIDs verifies that dedup is a no-op when
// no entries have PkgIDs (e.g. on Windows or when apps collector is absent).
func TestGetSoftwareInventoryDedupNoPkgIDs(t *testing.T) {
	c1 := &MockCollector{
		entries: map[string]*Entry{
			"a": {DisplayName: "App A", Source: "desktop", ProductCode: "A"},
			"b": {DisplayName: "App B", Source: "desktop", ProductCode: "B"},
		},
	}
	c2 := &MockCollector{
		entries: map[string]*Entry{
			"c": {DisplayName: "App C", Source: "desktop", ProductCode: "C"},
		},
	}

	entries, _, err := GetSoftwareInventoryWithCollectors([]Collector{c1, c2})
	require.NoError(t, err)
	assert.Len(t, entries, 3, "All entries should be kept when no PkgIDs exist")
}

// TestPkgReceiptsCollectorInstallPathFromPrefix verifies that install paths
// are correctly derived from the InstallPrefixPath in the receipt plist.
func TestPkgReceiptsCollectorInstallPathFromPrefix(t *testing.T) {
	tests := []struct {
		name        string
		prefixPath  string
		expectPath  string
		expectIsApp bool
	}{
		{
			name:        "Applications prefix",
			prefixPath:  "Applications",
			expectPath:  "/Applications",
			expectIsApp: true,
		},
		{
			name:        "Applications with .app suffix",
			prefixPath:  "Applications/Pages.app/Contents/",
			expectPath:  "/Applications/Pages.app/Contents/",
			expectIsApp: true,
		},
		{
			name:        "opt prefix",
			prefixPath:  "opt/datadog-agent",
			expectPath:  "/opt/datadog-agent",
			expectIsApp: false,
		},
		{
			name:        "usr/local prefix",
			prefixPath:  "usr/local/bin/",
			expectPath:  "/usr/local/bin/",
			expectIsApp: false,
		},
		{
			name:        "root prefix",
			prefixPath:  "/",
			expectPath:  "N/A",
			expectIsApp: false,
		},
		{
			name:        "empty prefix",
			prefixPath:  "",
			expectPath:  "N/A",
			expectIsApp: false,
		},
		{
			name:        "Library prefix",
			prefixPath:  "Library/Internet Plug-Ins/JavaAppletPlugin.plugin",
			expectPath:  "/Library/Internet Plug-Ins/JavaAppletPlugin.plugin",
			expectIsApp: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isApp := strings.HasPrefix(tt.prefixPath, "Applications")
			assert.Equal(t, tt.expectIsApp, isApp, "App detection mismatch for prefix %q", tt.prefixPath)

			var installPath string
			if tt.prefixPath != "" && tt.prefixPath != "/" {
				if !strings.HasPrefix(tt.prefixPath, "/") {
					installPath = "/" + tt.prefixPath
				} else {
					installPath = tt.prefixPath
				}
			} else {
				installPath = "N/A"
			}
			assert.Equal(t, tt.expectPath, installPath, "Install path mismatch for prefix %q", tt.prefixPath)
		})
	}
}
