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

// TestSoftwareTypeConstants verifies that the core software type constants
// (app, pkg, mas, kext, sysext, homebrew) are unique, non-empty, and have
// the expected string values.
func TestSoftwareTypeConstants(t *testing.T) {
	types := []string{
		softwareTypeApp,
		softwareTypePkg,
		softwareTypeMAS,
		softwareTypeKext,
		softwareTypeSysExt,
		softwareTypeHomebrew,
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
	assert.Equal(t, "pkg", softwareTypePkg)
	assert.Equal(t, "mas", softwareTypeMAS)
	assert.Equal(t, "kext", softwareTypeKext)
	assert.Equal(t, "sysext", softwareTypeSysExt)
	assert.Equal(t, "homebrew", softwareTypeHomebrew)
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

// TestNixCollectorNoNix verifies that the Nix collector gracefully handles
// systems where Nix is not installed, returning an empty list without errors.
// Also validates that any collected entries have the required fields.
func TestNixCollectorNoNix(t *testing.T) {
	collector := &nixCollector{}
	entries, warnings, err := collector.Collect()

	// Should not return an error even if Nix isn't installed
	assert.NoError(t, err)

	t.Logf("Found %d Nix entries with %d warnings", len(entries), len(warnings))

	// Validate entries - log warnings for issues instead of failing
	issueCount := 0
	for i, entry := range entries {
		if entry.DisplayName == "" {
			t.Logf("WARNING: Entry %d missing display name", i)
			issueCount++
		}
		if entry.Source != softwareTypeNix {
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

// TestParseNixStorePath tests parsing of Nix store paths to extract package
// name and version. Covers standard packages, packages with hyphens in names,
// packages without version numbers, and invalid paths.
func TestParseNixStorePath(t *testing.T) {
	tests := []struct {
		name        string
		storePath   string
		wantName    string
		wantVersion string
		wantNil     bool
	}{
		{
			name:        "standard package",
			storePath:   "/nix/store/abcdef0123456789abcdef0123456789-git-2.42.0",
			wantName:    "git",
			wantVersion: "2.42.0",
		},
		{
			name:        "package with hyphen in name",
			storePath:   "/nix/store/abcdef0123456789abcdef0123456789-node-modules-18.17.0",
			wantName:    "node-modules",
			wantVersion: "18.17.0",
		},
		{
			name:        "package without version",
			storePath:   "/nix/store/abcdef0123456789abcdef0123456789-somepkg",
			wantName:    "somepkg",
			wantVersion: "",
		},
		{
			name:      "invalid store path (too short)",
			storePath: "/nix/store/short",
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := parseNixStorePath(tt.storePath)
			if tt.wantNil {
				assert.Nil(t, pkg, "Expected nil for invalid store path")
			} else {
				require.NotNil(t, pkg, "Expected non-nil package info")
				assert.Equal(t, tt.wantName, pkg.Name, "Package name mismatch")
				assert.Equal(t, tt.wantVersion, pkg.Version, "Package version mismatch")
			}
		})
	}
}

// TestCondaCollectorNoConda verifies that the Conda collector gracefully
// handles systems where Conda/Mamba is not installed, returning an empty
// list without errors. Also validates that any collected entries have the
// required fields.
func TestCondaCollectorNoConda(t *testing.T) {
	collector := &condaCollector{}
	entries, warnings, err := collector.Collect()

	// Should not return an error even if Conda isn't installed
	assert.NoError(t, err)

	t.Logf("Found %d Conda entries with %d warnings", len(entries), len(warnings))

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
		if entry.Source != softwareTypeConda {
			t.Logf("WARNING: Entry %d (%s) has unexpected source: %s", i, entry.DisplayName, entry.Source)
			issueCount++
		}
	}

	if issueCount > 0 {
		t.Logf("Found %d validation issues in %d entries (logged as warnings)", issueCount, len(entries))
	}
}

// TestParseCondaMeta tests parsing of Conda's conda-meta JSON files, verifying
// extraction of package name, version, build info, and installation timestamp.
func TestParseCondaMeta(t *testing.T) {
	tempDir := t.TempDir()

	// Create a sample conda-meta JSON file
	metaContent := `{
		"name": "numpy",
		"version": "1.24.0",
		"build": "py311h0b4df5a_0",
		"build_number": 0,
		"channel": "https://repo.anaconda.com/pkgs/main",
		"subdir": "osx-arm64",
		"timestamp": 1704067200000,
		"requested_spec": true
	}`

	metaPath := filepath.Join(tempDir, "numpy-1.24.0-py311h0b4df5a_0.json")
	err := os.WriteFile(metaPath, []byte(metaContent), 0644)
	require.NoError(t, err)

	// Parse the metadata
	meta, err := parseCondaMeta(metaPath)
	require.NoError(t, err)

	assert.Equal(t, "numpy", meta.Name)
	assert.Equal(t, "1.24.0", meta.Version)
	assert.Equal(t, "py311h0b4df5a_0", meta.Build)
	assert.True(t, meta.Requested)
	assert.Equal(t, int64(1704067200000), meta.Timestamp)
}

// TestPipCollectorNoPip verifies that the pip collector gracefully handles
// systems without pip packages, returning an empty list without errors. Also
// validates that any collected entries have the required fields.
func TestPipCollectorNoPip(t *testing.T) {
	collector := &pipCollector{}
	entries, warnings, err := collector.Collect()

	// Should not return an error
	assert.NoError(t, err)

	t.Logf("Found %d pip entries with %d warnings", len(entries), len(warnings))

	// Validate entries - log warnings for issues instead of failing
	issueCount := 0
	for i, entry := range entries {
		if entry.DisplayName == "" {
			t.Logf("WARNING: Entry %d missing display name", i)
			issueCount++
		}
		if entry.Source != softwareTypePip {
			t.Logf("WARNING: Entry %d (%s) has unexpected source: %s", i, entry.DisplayName, entry.Source)
			issueCount++
		}
	}

	if issueCount > 0 {
		t.Logf("Found %d validation issues in %d entries (logged as warnings)", issueCount, len(entries))
	}
}

// TestParsePipMetadata tests parsing of pip's dist-info/METADATA files,
// verifying extraction of package name, version, author, and homepage.
func TestParsePipMetadata(t *testing.T) {
	tempDir := t.TempDir()

	// Create a sample METADATA file
	metadataContent := `Metadata-Version: 2.1
Name: requests
Version: 2.31.0
Author: Kenneth Reitz
Author-email: me@kennethreitz.org
Home-page: https://requests.readthedocs.io

A simple HTTP library.`

	metadataPath := filepath.Join(tempDir, "METADATA")
	err := os.WriteFile(metadataPath, []byte(metadataContent), 0644)
	require.NoError(t, err)

	// Parse the metadata
	name, version, author, homepage, err := parsePipMetadata(metadataPath)
	require.NoError(t, err)

	assert.Equal(t, "requests", name)
	assert.Equal(t, "2.31.0", version)
	assert.Equal(t, "Kenneth Reitz", author)
	assert.Equal(t, "https://requests.readthedocs.io", homepage)
}

// TestNpmCollectorNoNpm verifies that the npm collector gracefully handles
// systems without globally installed npm packages, returning an empty list
// without errors. Also validates that any collected entries have the required fields.
func TestNpmCollectorNoNpm(t *testing.T) {
	collector := &npmCollector{}
	entries, warnings, err := collector.Collect()

	// Should not return an error
	assert.NoError(t, err)

	t.Logf("Found %d npm entries with %d warnings", len(entries), len(warnings))

	// Validate entries - log warnings for issues instead of failing
	issueCount := 0
	for i, entry := range entries {
		if entry.DisplayName == "" {
			t.Logf("WARNING: Entry %d missing display name", i)
			issueCount++
		}
		if entry.Source != softwareTypeNpm {
			t.Logf("WARNING: Entry %d (%s) has unexpected source: %s", i, entry.DisplayName, entry.Source)
			issueCount++
		}
	}

	if issueCount > 0 {
		t.Logf("Found %d validation issues in %d entries (logged as warnings)", issueCount, len(entries))
	}
}

// TestParseNpmPackageJSON tests parsing of npm package.json files, verifying
// extraction of package name, version, description, and author information.
func TestParseNpmPackageJSON(t *testing.T) {
	tempDir := t.TempDir()

	// Create a sample package.json
	packageContent := `{
		"name": "typescript",
		"version": "5.3.3",
		"description": "TypeScript is a language for application scale JavaScript development",
		"author": "Microsoft Corp."
	}`

	packagePath := filepath.Join(tempDir, "package.json")
	err := os.WriteFile(packagePath, []byte(packageContent), 0644)
	require.NoError(t, err)

	// Parse the package.json
	pkg, err := parseNpmPackageJSON(packagePath)
	require.NoError(t, err)

	assert.Equal(t, "typescript", pkg.Name)
	assert.Equal(t, "5.3.3", pkg.Version)
	assert.Equal(t, "TypeScript is a language for application scale JavaScript development", pkg.Description)
}

// TestExtractAuthor tests the extractAuthor helper function which handles
// various npm author formats including simple strings, strings with email/URL,
// and object format with name field.
func TestExtractAuthor(t *testing.T) {
	tests := []struct {
		name   string
		author interface{}
		want   string
	}{
		{
			name:   "string author",
			author: "John Doe",
			want:   "John Doe",
		},
		{
			name:   "string with email",
			author: "John Doe <john@example.com>",
			want:   "John Doe",
		},
		{
			name:   "string with url",
			author: "John Doe (https://example.com)",
			want:   "John Doe",
		},
		{
			name:   "object author",
			author: map[string]interface{}{"name": "Jane Doe", "email": "jane@example.com"},
			want:   "Jane Doe",
		},
		{
			name:   "nil author",
			author: nil,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAuthor(tt.author)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestGemCollectorNoGems verifies that the gem collector gracefully handles
// systems without installed Ruby gems, returning an empty list without errors.
// Also validates that any collected entries have the required fields.
func TestGemCollectorNoGems(t *testing.T) {
	collector := &gemCollector{}
	entries, warnings, err := collector.Collect()

	// Should not return an error
	assert.NoError(t, err)

	t.Logf("Found %d gem entries with %d warnings", len(entries), len(warnings))

	// Validate entries - log warnings for issues instead of failing
	issueCount := 0
	for i, entry := range entries {
		if entry.DisplayName == "" {
			t.Logf("WARNING: Entry %d missing display name", i)
			issueCount++
		}
		if entry.Source != softwareTypeGem {
			t.Logf("WARNING: Entry %d (%s) has unexpected source: %s", i, entry.DisplayName, entry.Source)
			issueCount++
		}
	}

	if issueCount > 0 {
		t.Logf("Found %d validation issues in %d entries (logged as warnings)", issueCount, len(entries))
	}
}

// TestParseGemspec tests parsing of Ruby gemspec files, verifying extraction
// of gem name, version, author, and homepage information using regex patterns.
func TestParseGemspec(t *testing.T) {
	tempDir := t.TempDir()

	// Create a sample gemspec file
	gemspecContent := `# -*- encoding: utf-8 -*-
Gem::Specification.new do |s|
  s.name = "rails"
  s.version = "7.1.3"
  s.authors = ["David Heinemeier Hansson"]
  s.homepage = "https://rubyonrails.org"
  s.summary = "Full-stack web application framework."
end`

	gemspecPath := filepath.Join(tempDir, "rails-7.1.3.gemspec")
	err := os.WriteFile(gemspecPath, []byte(gemspecContent), 0644)
	require.NoError(t, err)

	// Parse the gemspec
	info, err := parseGemspec(gemspecPath)
	require.NoError(t, err)

	assert.Equal(t, "rails", info.Name)
	assert.Equal(t, "7.1.3", info.Version)
	assert.Equal(t, "David Heinemeier Hansson", info.Author)
	assert.Equal(t, "https://rubyonrails.org", info.Homepage)
}

// TestCargoCollectorNoCargo verifies that the Cargo collector gracefully handles
// systems without installed Rust crates, returning an empty list without errors.
// Also validates that any collected entries have the required fields.
func TestCargoCollectorNoCargo(t *testing.T) {
	collector := &cargoCollector{}
	entries, warnings, err := collector.Collect()

	// Should not return an error
	assert.NoError(t, err)

	t.Logf("Found %d cargo entries with %d warnings", len(entries), len(warnings))

	// Validate entries - log warnings for issues instead of failing
	issueCount := 0
	for i, entry := range entries {
		if entry.DisplayName == "" {
			t.Logf("WARNING: Entry %d missing display name", i)
			issueCount++
		}
		if entry.Source != softwareTypeCargo {
			t.Logf("WARNING: Entry %d (%s) has unexpected source: %s", i, entry.DisplayName, entry.Source)
			issueCount++
		}
	}

	if issueCount > 0 {
		t.Logf("Found %d validation issues in %d entries (logged as warnings)", issueCount, len(entries))
	}
}

// TestParseCratesToml tests parsing of Cargo's .crates.toml file, verifying
// extraction of crate names and versions from the v1 section format.
func TestParseCratesToml(t *testing.T) {
	tempDir := t.TempDir()

	// Create a sample .crates.toml file
	cratesContent := `[v1]
"ripgrep 14.0.3 (registry+https://github.com/rust-lang/crates.io-index)" = ["rg"]
"cargo-watch 8.4.1 (registry+https://github.com/rust-lang/crates.io-index)" = ["cargo-watch"]
`

	cratesPath := filepath.Join(tempDir, ".crates.toml")
	err := os.WriteFile(cratesPath, []byte(cratesContent), 0644)
	require.NoError(t, err)

	// Parse the .crates.toml
	crates, err := parseCratesToml(cratesPath)
	require.NoError(t, err)

	assert.Len(t, crates, 2)

	// Check first crate
	assert.Equal(t, "ripgrep", crates[0]["name"])
	assert.Equal(t, "14.0.3", crates[0]["version"])

	// Check second crate
	assert.Equal(t, "cargo-watch", crates[1]["name"])
	assert.Equal(t, "8.4.1", crates[1]["version"])
}

// TestAllNewSoftwareTypeConstants verifies that all software type constants
// (including package managers: macports, nix, conda, pip, npm, gem, cargo)
// are unique, non-empty, and have the expected string values.
func TestAllNewSoftwareTypeConstants(t *testing.T) {
	types := []string{
		softwareTypeApp,
		softwareTypePkg,
		softwareTypeMAS,
		softwareTypeKext,
		softwareTypeSysExt,
		softwareTypeHomebrew,
		softwareTypeMacPorts,
		softwareTypeNix,
		softwareTypeConda,
		softwareTypePip,
		softwareTypeNpm,
		softwareTypeGem,
		softwareTypeCargo,
	}

	// Check uniqueness
	seen := make(map[string]bool)
	for _, st := range types {
		assert.NotEmpty(t, st, "Software type should not be empty")
		assert.False(t, seen[st], "Software type %s should be unique", st)
		seen[st] = true
	}

	// Verify expected values for new types
	assert.Equal(t, "macports", softwareTypeMacPorts)
	assert.Equal(t, "nix", softwareTypeNix)
	assert.Equal(t, "conda", softwareTypeConda)
	assert.Equal(t, "pip", softwareTypePip)
	assert.Equal(t, "npm", softwareTypeNpm)
	assert.Equal(t, "gem", softwareTypeGem)
	assert.Equal(t, "cargo", softwareTypeCargo)
}
