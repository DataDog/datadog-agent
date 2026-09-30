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

// withSystemVersionPlist points the collector at a fixture for the duration of a test.
// Empty content leaves the file absent.
func withSystemVersionPlist(t *testing.T, content string) {
	t.Helper()

	original := systemVersionPlistPath
	t.Cleanup(func() { systemVersionPlistPath = original })

	systemVersionPlistPath = filepath.Join(t.TempDir(), "SystemVersion.plist")
	if content != "" {
		require.NoError(t, os.WriteFile(systemVersionPlistPath, []byte(content), 0o600))
	}
}

func TestOSCollectorReportsTheRunningSystem(t *testing.T) {
	withSystemVersionPlist(t, testSystemVersionPlist)

	entries, warnings, err := (&osCollector{}).Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, entries, 1, "the operating system is exactly one entry")

	entry := entries[0]
	assert.Equal(t, softwareTypeOS, entry.Source)
	assert.Equal(t, "OS", entry.DisplayName)
	assert.Equal(t, "os_version", entry.ProductCode)
	assert.Equal(t, "15.6 (24G84)", entry.Version)
	assert.Equal(t, applePublisher, entry.Publisher)
	assert.Equal(t, statusInstalled, entry.Status)
	assert.True(t, entry.Is64Bit)
	assert.Empty(t, entry.InstallDate, "nothing records when the running version was applied")
}

func TestOSCollectorIdentitySurvivesAnUpgrade(t *testing.T) {
	// The backend keys a software item on name and product code with the version
	// excluded, so an upgrade must change only the version. Anything version-derived in
	// the name or product code would read as an uninstall plus a fresh install.
	withSystemVersionPlist(t, testSystemVersionPlist)
	before, _, err := (&osCollector{}).Collect()
	require.NoError(t, err)
	require.Len(t, before, 1)

	withSystemVersionPlist(t, `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>ProductVersion</key>
	<string>26.4.1</string>
	<key>ProductBuildVersion</key>
	<string>25E253</string>
</dict>
</plist>`)
	after, _, err := (&osCollector{}).Collect()
	require.NoError(t, err)
	require.Len(t, after, 1)

	assert.Equal(t, before[0].DisplayName, after[0].DisplayName)
	assert.Equal(t, before[0].ProductCode, after[0].ProductCode)
	assert.Equal(t, before[0].GetID(), after[0].GetID())
	assert.NotEqual(t, before[0].Version, after[0].Version)
}

func TestOSCollectorMissingSystemVersionIsFatal(t *testing.T) {
	// macOS always runs some version, so an empty result would read as the operating
	// system having been removed.
	withSystemVersionPlist(t, "")

	entries, _, err := (&osCollector{}).Collect()

	assert.Error(t, err)
	assert.Empty(t, entries)
}

func TestOSCollectorRejectsAPlistWithoutProductVersion(t *testing.T) {
	withSystemVersionPlist(t, `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>ProductName</key>
	<string>macOS</string>
</dict>
</plist>`)

	_, _, err := (&osCollector{}).Collect()

	assert.Error(t, err, "a system version with no ProductVersion is unusable")
}

func TestOSVersionString(t *testing.T) {
	tests := []struct {
		name           string
		productVersion string
		buildVersion   string
		expected       string
	}{
		{
			// The build is not numeric, so it goes in parentheses rather than as a
			// fourth dot-separated component: a dotted form would look orderable while
			// comparing wrongly.
			name:           "the build is appended in parentheses",
			productVersion: "15.6",
			buildVersion:   "24G84",
			expected:       "15.6 (24G84)",
		},
		{
			name:           "a missing build leaves the product version alone",
			productVersion: "15.6",
			buildVersion:   "",
			expected:       "15.6",
		},
		{
			name:           "surrounding whitespace is trimmed",
			productVersion: "15.6",
			buildVersion:   "  24G84\n",
			expected:       "15.6 (24G84)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, osVersionString(test.productVersion, test.buildVersion))
		})
	}
}

func TestOSVersionStringDistinguishesSupplementalBuilds(t *testing.T) {
	// The reason the build is reported at all: Apple ships supplemental updates and
	// hardware-specific builds that advance the build while leaving the product version
	// unchanged, and a Rapid Security Response does too.
	assert.NotEqual(t,
		osVersionString("15.6", "24G84"),
		osVersionString("15.6", "24G90"),
	)
}
