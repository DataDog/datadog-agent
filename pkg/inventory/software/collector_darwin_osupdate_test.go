// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSystemVersionPlist = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>ProductName</key>
	<string>macOS</string>
	<key>ProductVersion</key>
	<string>15.6</string>
	<key>ProductBuildVersion</key>
	<string>24G84</string>
</dict>
</plist>`

const testInstallHistoryPlist = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<array>
	<dict>
		<key>date</key>
		<date>2025-06-10T12:30:15Z</date>
		<key>displayName</key>
		<string>macOS 15.6</string>
		<key>displayVersion</key>
		<string>15.6</string>
		<key>packageIdentifiers</key>
		<array>
			<string>com.apple.pkg.update</string>
		</array>
		<key>processName</key>
		<string>softwareupdated</string>
	</dict>
	<dict>
		<key>date</key>
		<date>2025-02-01T09:00:00Z</date>
		<key>displayName</key>
		<string>Security Response 15.5 (a)</string>
		<key>displayVersion</key>
		<string>15.5</string>
		<key>processName</key>
		<string>installer</string>
	</dict>
	<dict>
		<key>date</key>
		<date>2025-03-01T09:00:00Z</date>
		<key>displayName</key>
		<string>Some Random App</string>
		<key>displayVersion</key>
		<string>3.1</string>
		<key>processName</key>
		<string>installer</string>
	</dict>
</array>
</plist>`

// withOSUpdatePlists points the collector at fixture files for the duration of a test.
// An empty path leaves the file absent.
func withOSUpdatePlists(t *testing.T, systemVersion, installHistory string) {
	t.Helper()

	dir := t.TempDir()

	origSystemVersion, origInstallHistory := systemVersionPlistPath, installHistoryPlistPath
	t.Cleanup(func() {
		systemVersionPlistPath, installHistoryPlistPath = origSystemVersion, origInstallHistory
	})

	systemVersionPlistPath = filepath.Join(dir, "SystemVersion.plist")
	if systemVersion != "" {
		require.NoError(t, os.WriteFile(systemVersionPlistPath, []byte(systemVersion), 0o600))
	}

	installHistoryPlistPath = filepath.Join(dir, "InstallHistory.plist")
	if installHistory != "" {
		require.NoError(t, os.WriteFile(installHistoryPlistPath, []byte(installHistory), 0o600))
	}
}

func TestOSUpdateCollectorEmitsRunningSystem(t *testing.T) {
	withOSUpdatePlists(t, testSystemVersionPlist, testInstallHistoryPlist)

	entries, warnings, err := (&osUpdateCollector{}).Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.NotEmpty(t, entries)

	system := entries[0]
	assert.Equal(t, macOSProductCode, system.ProductCode)
	assert.Equal(t, "macOS 15.6", system.DisplayName)
	assert.Equal(t, "15.6", system.Version)
	assert.Equal(t, softwareTypeOSUpdate, system.Source)
	assert.Equal(t, applePublisher, system.Publisher)
	assert.Equal(t, statusInstalled, system.Status)
}

func TestOSUpdateCollectorSurvivesMissingInstallHistory(t *testing.T) {
	withOSUpdatePlists(t, testSystemVersionPlist, "")

	entries, warnings, err := (&osUpdateCollector{}).Collect()

	require.NoError(t, err, "a missing install history must not fail the snapshot")
	require.Len(t, entries, 1)
	assert.Equal(t, macOSProductCode, entries[0].ProductCode)
	assert.Len(t, warnings, 1)
}

func TestOSUpdateCollectorMissingSystemVersionIsFatal(t *testing.T) {
	withOSUpdatePlists(t, "", testInstallHistoryPlist)

	entries, _, err := (&osUpdateCollector{}).Collect()

	assert.Error(t, err, "macOS always runs some version, so this can never be legitimately empty")
	assert.Empty(t, entries)
}

func TestOSUpdateCollectorRejectsSystemVersionWithoutProductVersion(t *testing.T) {
	withOSUpdatePlists(t, `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>ProductName</key>
	<string>macOS</string>
</dict>
</plist>`, testInstallHistoryPlist)

	_, _, err := (&osUpdateCollector{}).Collect()

	assert.Error(t, err)
}

func TestOSUpdateCollectorFiltersInstallHistory(t *testing.T) {
	withOSUpdatePlists(t, testSystemVersionPlist, testInstallHistoryPlist)

	entries, _, err := (&osUpdateCollector{}).Collect()
	require.NoError(t, err)

	byProductCode := make(map[string]*Entry, len(entries))
	for _, entry := range entries {
		byProductCode[entry.ProductCode] = entry
	}

	update, ok := byProductCode["macos-15-6"]
	require.True(t, ok, "softwareupdated receipts are OS updates")
	assert.Equal(t, "macOS 15.6", update.DisplayName)
	assert.Equal(t, "15.6", update.Version)
	assert.Equal(t, "2025-06-10T12:30:15Z", update.InstallDate)

	_, ok = byProductCode["security-response-15-5-a"]
	assert.True(t, ok, "OS-update display name prefixes are recognized regardless of process")

	_, ok = byProductCode["some-random-app"]
	assert.False(t, ok, "ordinary application installs are not OS updates")
}

func TestOSUpdateCollectorDedupesRepeatedReceipts(t *testing.T) {
	withOSUpdatePlists(t, testSystemVersionPlist, `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<array>
	<dict>
		<key>date</key>
		<date>2025-01-01T00:00:00Z</date>
		<key>displayName</key>
		<string>macOS 15.6</string>
		<key>displayVersion</key>
		<string>15.6</string>
		<key>processName</key>
		<string>softwareupdated</string>
	</dict>
	<dict>
		<key>date</key>
		<date>2025-06-10T12:30:15Z</date>
		<key>displayName</key>
		<string>macOS 15.6</string>
		<key>displayVersion</key>
		<string>15.6</string>
		<key>processName</key>
		<string>softwareupdated</string>
	</dict>
</array>
</plist>`)

	entries, _, err := (&osUpdateCollector{}).Collect()
	require.NoError(t, err)

	var matched []*Entry
	for _, entry := range entries {
		if entry.ProductCode == "macos-15-6" {
			matched = append(matched, entry)
		}
	}

	require.Len(t, matched, 1)
	assert.Equal(t, "2025-06-10T12:30:15Z", matched[0].InstallDate, "the most recent receipt wins")
}

func TestOSUpdateCollectorWarnsOnMalformedReceipt(t *testing.T) {
	withOSUpdatePlists(t, testSystemVersionPlist, `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<array>
	<dict>
		<key>date</key>
		<date>2025-06-10T12:30:15Z</date>
		<key>processName</key>
		<string>softwareupdated</string>
	</dict>
</array>
</plist>`)

	entries, warnings, err := (&osUpdateCollector{}).Collect()

	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the running-system entry survives")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Message, "no display name")
}

func TestIsOSUpdateReceipt(t *testing.T) {
	tests := []struct {
		processName string
		displayName string
		want        bool
	}{
		{"softwareupdated", "Anything At All", true},
		{"installer", "macOS 15.6", true},
		{"installer", "Security Update 2025-004", true},
		{"installer", "Security Response 15.5 (a)", true},
		{"installer", "Some Random App", false},
		{"storedownloadd", "Xcode", false},
		{"", "", false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, isOSUpdateReceipt(tt.processName, tt.displayName),
			"isOSUpdateReceipt(%q, %q)", tt.processName, tt.displayName)
	}
}

func TestSlugifyDisplayName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"macOS 15.6", "macos-15-6"},
		{"Security Update 2025-004", "security-update-2025-004"},
		{"Security Response 15.5 (a)", "security-response-15-5-a"},
		{"  ", ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, slugifyDisplayName(tt.input), "slugifyDisplayName(%q)", tt.input)
	}
}

func TestParsePlistArrayToMaps(t *testing.T) {
	maps, err := parsePlistArrayToMaps([]byte(testInstallHistoryPlist))

	require.NoError(t, err)
	require.Len(t, maps, 3)

	assert.Equal(t, "macOS 15.6", maps[0]["displayName"])
	assert.Equal(t, "softwareupdated", maps[0]["processName"])
	assert.Equal(t, "2025-06-10T12:30:15Z", maps[0]["date"])
	assert.Equal(t, "15.6", maps[0]["displayVersion"])
	// The nested packageIdentifiers array must not leak into the element's values
	assert.NotContains(t, maps[0], "packageIdentifiers")

	assert.Equal(t, "Some Random App", maps[2]["displayName"])
}
