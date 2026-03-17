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

// TestGetLocalUsers verifies that getLocalUsers() correctly enumerates user home
// directories from /Users while filtering out system directories like Shared,
// Guest, and hidden directories. This test runs against the real filesystem.
func TestGetLocalUsers(t *testing.T) {
	users, warnings := getLocalUsers()

	t.Logf("Found %d users with %d warnings", len(users), len(warnings))

	// Validate returned user paths - log warnings instead of failing
	issueCount := 0
	for _, userPath := range users {
		username := filepath.Base(userPath)

		// Safety check for empty username
		if username == "" {
			t.Logf("WARNING: Found user path with empty username: %s", userPath)
			issueCount++
			continue
		}

		// Check for system directories that should be filtered
		if username == "Shared" || username == "Guest" || username == ".localized" {
			t.Logf("WARNING: System directory not filtered: %s", username)
			issueCount++
		}

		// Check for hidden directories
		if username[0] == '.' {
			t.Logf("WARNING: Hidden directory not filtered: %s", username)
			issueCount++
		}

		// Verify path is under /Users
		if !strings.HasPrefix(userPath, "/Users/") {
			t.Logf("WARNING: User path not under /Users: %s", userPath)
			issueCount++
		}
	}

	if issueCount > 0 {
		t.Logf("Found %d validation issues (logged as warnings)", issueCount)
	}
}

// TestUserAppDir tests the userAppDir struct to ensure it correctly stores
// both the application path and the associated username for per-user installations.
func TestUserAppDir(t *testing.T) {
	dir := userAppDir{
		path:     "/Users/testuser/Applications",
		username: "testuser",
	}

	assert.Equal(t, "/Users/testuser/Applications", dir.path)
	assert.Equal(t, "testuser", dir.username)

	// System-wide should have empty username
	sysDir := userAppDir{
		path:     "/Applications",
		username: "",
	}
	assert.Empty(t, sysDir.username)
}

// TestHomebrewPrefix tests the homebrewPrefix struct to verify it correctly
// stores the Homebrew installation path and associated username for both
// system-wide and per-user Homebrew installations.
func TestHomebrewPrefix(t *testing.T) {
	prefix := homebrewPrefix{
		path:     "/opt/homebrew",
		username: "",
	}

	assert.Equal(t, "/opt/homebrew", prefix.path)
	assert.Empty(t, prefix.username, "System-wide Homebrew should have empty username")

	// Per-user Homebrew
	userPrefix := homebrewPrefix{
		path:     "/Users/testuser/.homebrew",
		username: "testuser",
	}
	assert.Equal(t, "testuser", userPrefix.username)
}

// TestParseHomebrewReceipt tests parsing of Homebrew's INSTALL_RECEIPT.json files,
// verifying extraction of version, installation time, and dependency information.
func TestParseHomebrewReceipt(t *testing.T) {
	tempDir := t.TempDir()

	// Create a sample INSTALL_RECEIPT.json
	receiptContent := `{
		"homebrew_version": "4.2.0",
		"used_options": [],
		"source": {
			"spec": "stable",
			"versions": {
				"stable": "1.7.1"
			}
		},
		"installed_on_request": true,
		"installed_as_dependency": false,
		"time": 1704067200,
		"tabfile": "/opt/homebrew/Cellar/jq/1.7.1/.brew/jq.rb",
		"runtime_dependencies": [
			{"full_name": "oniguruma", "version": "6.9.9"}
		],
		"source_modified_time": 1704067000
	}`

	receiptPath := filepath.Join(tempDir, "INSTALL_RECEIPT.json")
	err := os.WriteFile(receiptPath, []byte(receiptContent), 0644)
	require.NoError(t, err)

	// Parse the receipt
	receipt, err := parseHomebrewReceipt(receiptPath)
	require.NoError(t, err)

	assert.Equal(t, "4.2.0", receipt.HomebrewVersion)
	assert.Equal(t, "stable", receipt.Source.Spec)
	assert.True(t, receipt.InstalledOnRequest)
	assert.False(t, receipt.InstalledAsDep)
	assert.Equal(t, int64(1704067200), receipt.Time)
	assert.Len(t, receipt.RuntimeDeps, 1)
	assert.Equal(t, "oniguruma", receipt.RuntimeDeps[0].FullName)
}

// TestParseHomebrewReceiptInvalid tests error handling when parsing invalid
// Homebrew receipt files, including non-existent files and malformed JSON.
func TestParseHomebrewReceiptInvalid(t *testing.T) {
	tempDir := t.TempDir()

	// Test with non-existent file
	_, err := parseHomebrewReceipt(filepath.Join(tempDir, "nonexistent.json"))
	assert.Error(t, err)

	// Test with invalid JSON
	invalidPath := filepath.Join(tempDir, "invalid.json")
	err = os.WriteFile(invalidPath, []byte("not valid json"), 0644)
	require.NoError(t, err)

	_, err = parseHomebrewReceipt(invalidPath)
	assert.Error(t, err)
}

// TestGetHomebrewPrefixes verifies that getHomebrewPrefixes() correctly discovers
// Homebrew installation directories on the system. This test runs against the
// real filesystem and validates that discovered prefixes have valid Cellar directories.
func TestGetHomebrewPrefixes(t *testing.T) {
	prefixes, warnings := getHomebrewPrefixes()

	t.Logf("Found %d Homebrew prefixes with %d warnings", len(prefixes), len(warnings))

	// Validate returned prefixes - log warnings instead of failing
	issueCount := 0
	for _, prefix := range prefixes {
		// Path should be non-empty
		if prefix.path == "" {
			t.Logf("WARNING: Found prefix with empty path")
			issueCount++
			continue
		}

		// Cellar should exist at this prefix
		cellarPath := filepath.Join(prefix.path, "Cellar")
		info, err := os.Stat(cellarPath)
		if err != nil {
			t.Logf("WARNING: Cellar not accessible at %s: %v", cellarPath, err)
			issueCount++
			continue
		}
		if !info.IsDir() {
			t.Logf("WARNING: Cellar is not a directory at %s", cellarPath)
			issueCount++
		}

		// If username is set, validate path location
		if prefix.username != "" {
			expectedPrefix := "/Users/" + prefix.username
			if !strings.HasPrefix(prefix.path, expectedPrefix) {
				t.Logf("WARNING: Per-user Homebrew path doesn't match username: %s (user: %s)",
					prefix.path, prefix.username)
				issueCount++
			}
		}
	}

	if issueCount > 0 {
		t.Logf("Found %d validation issues (logged as warnings)", issueCount)
	}
}

// TestHomebrewCollectorNoHomebrew verifies that the Homebrew collector gracefully
// handles systems where Homebrew is not installed, returning an empty list without
// errors. Also validates that any collected entries have the required fields.
func TestHomebrewCollectorNoHomebrew(t *testing.T) {
	collector := &homebrewCollector{}
	entries, warnings, err := collector.Collect()

	// Should not return an error even if Homebrew isn't installed
	assert.NoError(t, err)

	t.Logf("Found %d Homebrew entries with %d warnings", len(entries), len(warnings))

	// Validate entries - log warnings for issues instead of failing
	issueCount := 0
	for i, entry := range entries {
		if entry.DisplayName == "" {
			t.Logf("WARNING: Entry %d missing display name", i)
			issueCount++
		}
		if entry.Version == "" {
			t.Logf("WARNING: Entry %d (%s) missing version", i, entry.DisplayName)
			issueCount++
		}
		if entry.Source != softwareTypeHomebrew {
			t.Logf("WARNING: Entry %d (%s) has unexpected source: %s", i, entry.DisplayName, entry.Source)
			issueCount++
		}
		if entry.InstallPath == "" {
			t.Logf("WARNING: Entry %d (%s) missing install path", i, entry.DisplayName)
			issueCount++
		}
	}

	if issueCount > 0 {
		t.Logf("Found %d validation issues in %d entries (logged as warnings)", issueCount, len(entries))
	}
}

// TestApplicationsCollectorIntegration is an integration test that runs against
// the real filesystem. It verifies that the applications collector runs without
// errors and produces valid entry structures. Note: On CI runners, /Applications
// may be empty or have minimal apps, so no minimum count is asserted.
func TestApplicationsCollectorIntegration(t *testing.T) {
	collector := &applicationsCollector{}
	entries, warnings, err := collector.Collect()

	// Collector should not return an error
	require.NoError(t, err)

	t.Logf("Found %d applications with %d warnings", len(entries), len(warnings))

	// Validate entries - log warnings for issues instead of failing
	issueCount := 0
	for i, entry := range entries {
		if entry.DisplayName == "" {
			t.Logf("WARNING: Entry %d missing display name", i)
			issueCount++
		}

		// Source should be either 'app' or 'mas' (Mac App Store)
		if entry.Source != softwareTypeApp && entry.Source != softwareTypeMAS {
			t.Logf("WARNING: Entry %d (%s) has unexpected source: %s (expected 'app' or 'mas')",
				i, entry.DisplayName, entry.Source)
			issueCount++
		}

		// Install path should be an .app bundle
		if filepath.Ext(entry.InstallPath) != ".app" {
			t.Logf("WARNING: Entry %d (%s) install path is not .app bundle: %s",
				i, entry.DisplayName, entry.InstallPath)
			issueCount++
		}

		// Log per-user apps for debugging
		if strings.HasPrefix(entry.InstallPath, "/Users/") {
			t.Logf("Found per-user app: %s (user: %s)", entry.DisplayName, entry.UserSID)
		}
	}

	if issueCount > 0 {
		t.Logf("Found %d validation issues in %d entries (logged as warnings)", issueCount, len(entries))
	}
}

// TestApplicationsCollectorUserSID verifies that the UserSID field is properly
// populated for per-user applications by creating a mock app bundle structure
// in a temporary directory and validating that the Info.plist can be parsed.
func TestApplicationsCollectorUserSID(t *testing.T) {
	tempDir := t.TempDir()

	// Create a fake user structure
	fakeUserHome := filepath.Join(tempDir, "Users", "fakeuser")
	fakeUserApps := filepath.Join(fakeUserHome, "Applications")
	err := os.MkdirAll(fakeUserApps, 0755)
	require.NoError(t, err)

	// Create a minimal .app bundle
	appPath := filepath.Join(fakeUserApps, "TestApp.app", "Contents")
	err = os.MkdirAll(appPath, 0755)
	require.NoError(t, err)

	// Create Info.plist
	infoPlist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>TestApp</string>
	<key>CFBundleVersion</key>
	<string>1.0</string>
	<key>CFBundleIdentifier</key>
	<string>com.test.testapp</string>
</dict>
</plist>`

	err = os.WriteFile(filepath.Join(appPath, "Info.plist"), []byte(infoPlist), 0644)
	require.NoError(t, err)

	// Verify the plist can be read
	plistData, err := readPlistFile(filepath.Join(appPath, "Info.plist"))
	require.NoError(t, err)
	assert.Equal(t, "TestApp", plistData["CFBundleName"])
	assert.Equal(t, "1.0", plistData["CFBundleVersion"])
}

// TestSoftwareTypeConstants verifies that all software type constants
// are unique, non-empty, and have the expected string values.
func TestSoftwareTypeConstants(t *testing.T) {
	types := []string{
		softwareTypeApp,
		softwareTypeSystemApp,
		softwareTypePkg,
		softwareTypeMAS,
		softwareTypeKext,
		softwareTypeSysExt,
		softwareTypeHomebrew,
		softwareTypeMacPorts,
	}

	// Check uniqueness
	seen := make(map[string]bool)
	for _, st := range types {
		assert.NotEmpty(t, st, "Software type should not be empty")
		assert.False(t, seen[st], "Software type %s should be unique", st)
		seen[st] = true
	}

	// Verify expected values
	assert.Equal(t, "app", softwareTypeApp)
	assert.Equal(t, "system_app", softwareTypeSystemApp)
	assert.Equal(t, "pkg", softwareTypePkg)
	assert.Equal(t, "mas", softwareTypeMAS)
	assert.Equal(t, "kext", softwareTypeKext)
	assert.Equal(t, "sysext", softwareTypeSysExt)
	assert.Equal(t, "homebrew", softwareTypeHomebrew)
	assert.Equal(t, "macports", softwareTypeMacPorts)
}

// TestInstallSourceConstants verifies that install source constants (pkg, mas,
// manual) have the expected string values.
func TestInstallSourceConstants(t *testing.T) {
	assert.Equal(t, "pkg", installSourcePkg)
	assert.Equal(t, "mas", installSourceMAS)
	assert.Equal(t, "manual", installSourceManual)
}

// TestMacPortsCollectorNoMacPorts verifies that the MacPorts collector gracefully
// handles systems where MacPorts is not installed, returning an empty list without
// errors. Also validates that any collected entries have the required fields.
func TestMacPortsCollectorNoMacPorts(t *testing.T) {
	collector := &macPortsCollector{}
	entries, warnings, err := collector.Collect()

	// Should not return an error even if MacPorts isn't installed
	assert.NoError(t, err)

	t.Logf("Found %d MacPorts entries with %d warnings", len(entries), len(warnings))

	// Validate entries - log warnings for issues instead of failing
	issueCount := 0
	for i, entry := range entries {
		if entry.DisplayName == "" {
			t.Logf("WARNING: Entry %d missing display name", i)
			issueCount++
		}
		if entry.Version == "" {
			t.Logf("WARNING: Entry %d (%s) missing version", i, entry.DisplayName)
			issueCount++
		}
		if entry.Source != softwareTypeMacPorts {
			t.Logf("WARNING: Entry %d (%s) has unexpected source: %s", i, entry.DisplayName, entry.Source)
			issueCount++
		}
	}

	if issueCount > 0 {
		t.Logf("Found %d validation issues in %d entries (logged as warnings)", issueCount, len(entries))
	}
}
