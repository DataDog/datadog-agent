// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package software

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockCollector implements Collector for testing
type MockCollector struct {
	entries  map[string]*Entry
	warnings []*Warning
	err      error
}

// SlowCollector returns only after delay. It is a distinct type from MockCollector so a
// test can pair a collector that misses its deadline with one that does not: the in-flight
// guard keys on the collector's type.
type SlowCollector struct {
	delay time.Duration
}

func (s *SlowCollector) Collect() ([]*Entry, []*Warning, error) {
	time.Sleep(s.delay)
	return []*Entry{{DisplayName: "Slow App", Source: "desktop"}}, nil, nil
}

// BlockingCollector blocks until release is closed, modelling a native call that has hung.
type BlockingCollector struct {
	release chan struct{}
}

func (b *BlockingCollector) Collect() ([]*Entry, []*Warning, error) {
	<-b.release
	return []*Entry{{DisplayName: "Blocking App", Source: "desktop"}}, nil, nil
}

func (m *MockCollector) Collect() ([]*Entry, []*Warning, error) {
	if m.err != nil {
		return nil, m.warnings, m.err
	}
	var result []*Entry
	var warnings []*Warning
	for _, entry := range m.entries {
		if entry != nil {
			result = append(result, entry)
		} else {
			warnings = append(warnings, warnf("invalid software detected"))
		}
	}
	warnings = append(warnings, m.warnings...)
	return result, warnings, nil
}

func TestCollectorOrchestration(t *testing.T) {
	tests := []struct {
		name                string
		collectors          []Collector
		expectedEntryCount  int
		expectedWarningMsgs []string
		expectError         bool
	}{
		{
			name: "Multiple collectors with overlapping data",
			collectors: []Collector{
				&MockCollector{
					entries: map[string]*Entry{
						"app1": {DisplayName: "App 1", Version: "1.0", Source: "desktop"},
						"app2": {DisplayName: "App 2", Version: "2.0", Source: "desktop"},
					},
				},
				&MockCollector{
					entries: map[string]*Entry{
						"app1": {DisplayName: "App 1", Version: "1.0", Source: "desktop"}, // Same app, different source
						"app3": {DisplayName: "App 3", Version: "3.0", Source: "desktop"},
					},
				},
			},
			expectedEntryCount: 4, // Should include both versions of App 1
		},
		{
			name: "Collector with mixed valid and invalid entries",
			collectors: []Collector{
				&MockCollector{
					entries: map[string]*Entry{
						"valid":   {DisplayName: "Valid App", Version: "1.0", Source: "desktop"},
						"invalid": nil, // This should generate a warning
					},
				},
			},
			expectedEntryCount:  1,
			expectedWarningMsgs: []string{"invalid software detected"},
		},
		{
			name: "Collector error handling - continues with other collectors",
			collectors: []Collector{
				&MockCollector{
					err: errors.New("registry access denied"),
				},
				&MockCollector{
					entries: map[string]*Entry{
						"app1": {DisplayName: "MSI App", Version: "1.0", Source: "desktop"},
					},
				},
			},
			expectedEntryCount: 1, // Should still get MSI entries despite registry error
			expectError:        true,
		},
		{
			name: "Collector error handling - multiple errors",
			collectors: []Collector{
				&MockCollector{
					err: errors.New("msi error"),
					entries: map[string]*Entry{
						"app1": {DisplayName: "MSI App", Version: "1.0", Source: "desktop"},
					},
				},
				&MockCollector{err: errors.New("registry error")},
			},
			expectedEntryCount: 0, // No entries returned on error because the collector was skipped
			expectError:        true,
		},
		{
			name: "Warning aggregation from multiple sources",
			collectors: []Collector{
				&MockCollector{
					entries: map[string]*Entry{
						"app1": {DisplayName: "Registry App", Version: "1.0", Source: "desktop"},
					},
					warnings: []*Warning{warnf("registry warning 1"), warnf("registry warning 2")},
				},
				&MockCollector{
					entries: map[string]*Entry{
						"app2": {DisplayName: "MSI App", Version: "1.0", Source: "desktop"},
					},
					warnings: []*Warning{warnf("msi warning 1")},
				},
			},
			expectedEntryCount:  2,
			expectedWarningMsgs: []string{"registry warning 1", "registry warning 2", "msi warning 1"},
		},
		{
			name: "Empty collectors",
			collectors: []Collector{
				// In both cases mock collectors return empty entries
				&MockCollector{entries: map[string]*Entry{}},
				&MockCollector{entries: nil},
			},
			expectedEntryCount: 0,
		},
		{
			name:               "No collectors provided",
			collectors:         []Collector{},
			expectedEntryCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inventory, warnings, err := GetSoftwareInventoryWithCollectors(tt.collectors)

			// Verify error expectation
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify entry count
			assert.Len(t, inventory, tt.expectedEntryCount,
				"Expected %d entries but got %d", tt.expectedEntryCount, len(inventory))

			// Verify expected warnings
			if len(tt.expectedWarningMsgs) > 0 {
				assert.Len(t, warnings, len(tt.expectedWarningMsgs),
					"Expected %d warnings but got %d", len(tt.expectedWarningMsgs), len(warnings))

				for i, expectedMsg := range tt.expectedWarningMsgs {
					assert.Contains(t, warnings[i].Message, expectedMsg,
						"Warning %d should contain '%s'", i, expectedMsg)
				}
			} else {
				assert.Empty(t, warnings, "Expected no warnings but got %v", warnings)
			}

			// Verify all entries are non-nil
			for i, entry := range inventory {
				assert.NotNil(t, entry, "Entry %d should not be nil", i)
				assert.NotEmpty(t, entry.DisplayName, "Entry %d should have a display name", i)
			}
		})
	}
}

func TestCollectorDeadline(t *testing.T) {
	const timeout = 50 * time.Millisecond

	t.Run("Slow collector fails the snapshot but keeps other entries", func(t *testing.T) {
		slow := &SlowCollector{delay: 10 * timeout}
		fast := &MockCollector{
			entries: map[string]*Entry{"fast": {DisplayName: "Fast App", Source: "desktop"}},
		}

		inventory, _, err := getSoftwareInventory([]Collector{slow, fast}, timeout)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
		assert.Len(t, inventory, 1, "entries from collectors that finished should survive")
		assert.Equal(t, "Fast App", inventory[0].DisplayName)
	})

	t.Run("Collector finishing within the deadline succeeds", func(t *testing.T) {
		collector := &MockCollector{
			entries: map[string]*Entry{"app": {DisplayName: "App", Source: "desktop"}},
		}

		inventory, _, err := getSoftwareInventory([]Collector{collector}, timeout)

		assert.NoError(t, err)
		assert.Len(t, inventory, 1)
	})

	t.Run("Successful empty collector is not an error", func(t *testing.T) {
		// An enumeration that legitimately finds nothing must not fail the
		// snapshot; only failures and timeouts do.
		inventory, _, err := getSoftwareInventory([]Collector{&MockCollector{}}, timeout)

		assert.NoError(t, err)
		assert.Empty(t, inventory)
	})

	t.Run("A timed-out collector is not started again while it is still running", func(t *testing.T) {
		// A hung native call cannot be cancelled, so repeated collections must not stack
		// up blocked goroutines holding OS handles.
		blocking := &BlockingCollector{release: make(chan struct{})}

		_, _, err := getSoftwareInventory([]Collector{blocking}, timeout)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")

		_, _, err = getSoftwareInventory([]Collector{blocking}, timeout)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "still running from a previous collection")

		// Once the blocked call returns, the collector becomes usable again.
		close(blocking.release)
		assert.Eventually(t, func() bool {
			inventory, _, err := getSoftwareInventory([]Collector{blocking}, timeout)
			return err == nil && len(inventory) == 1
		}, time.Second, 10*time.Millisecond)
	})
}

func TestEntryIDsAreUniqueAcrossSources(t *testing.T) {
	// os and driver entries share the snapshot with application entries;
	// GetID must keep them distinct so the backend does not collapse them.
	collector := &MockCollector{
		entries: map[string]*Entry{
			"driver":   {DisplayName: "Wi-Fi Adapter", ProductCode: "wdfilter", Source: softwareTypeDriver},
			"os":       {DisplayName: osDisplayName, ProductCode: osProductCode, Source: softwareTypeOS},
			"desktop":  {DisplayName: "Wi-Fi Adapter", ProductCode: "wdfilter", Source: "desktop"},
			"homebrew": {DisplayName: "git", ProductCode: "git", Source: "homebrew"},
		},
	}

	inventory, _, err := GetSoftwareInventoryWithCollectors([]Collector{collector})
	assert.NoError(t, err)

	seen := make(map[string]struct{}, len(inventory))
	for _, entry := range inventory {
		id := entry.GetID()
		_, duplicate := seen[id]
		assert.False(t, duplicate, "duplicate entry ID %q", id)
		seen[id] = struct{}{}
	}
	assert.Len(t, seen, len(inventory))
}

func TestWarnings(t *testing.T) {
	w := warnf("test %s %d", "warning", 123)
	assert.Equal(t, "test warning 123", w.Message)

	warn := Warning{Message: "test warning"}
	assert.Equal(t, "test warning", warn.Message)
}

func TestPrivateFieldsExcludedFromJSON(t *testing.T) {
	// Test that private fields (with json:"-") are excluded from JSON serialization
	// but still accessible in Go code
	entry := &Entry{
		DisplayName:   "TestApp",
		Version:       "1.0",
		Source:        "app",
		ProductCode:   "com.test.app",
		BrokenReason:  "executable not found",
		InstallSource: "pkg",
		PkgID:         "com.test.pkg",
		InstallPath:   "/Applications/TestApp.app",
		InstallPaths:  []string{"/Applications", "/Library"},
	}

	// Verify fields are accessible in Go code
	assert.Equal(t, "executable not found", entry.BrokenReason)
	assert.Equal(t, "pkg", entry.InstallSource)
	assert.Equal(t, "com.test.pkg", entry.PkgID)
	assert.Equal(t, "/Applications/TestApp.app", entry.InstallPath)
	assert.Equal(t, []string{"/Applications", "/Library"}, entry.InstallPaths)

	// Marshal to JSON
	jsonData, err := json.Marshal(entry)
	assert.NoError(t, err)

	jsonStr := string(jsonData)

	// Verify private fields are NOT in JSON. install_path is checked as a quoted
	// key because it is a substring of the exposed install_paths.
	assert.NotContains(t, jsonStr, "broken_reason")
	assert.NotContains(t, jsonStr, "install_source")
	assert.NotContains(t, jsonStr, "pkg_id")
	assert.NotContains(t, jsonStr, `"install_path"`)

	// Verify public fields ARE in JSON
	assert.Contains(t, jsonStr, "software_type")
	assert.Contains(t, jsonStr, "name")
	assert.Contains(t, jsonStr, "version")
	assert.Contains(t, jsonStr, "product_code")
	assert.Contains(t, jsonStr, `"install_paths"`)
	assert.Contains(t, jsonStr, "/Applications")
	assert.Contains(t, jsonStr, "/Library")
}

func TestInstallPathsMirrorsInstallPath(t *testing.T) {
	// GetSoftwareInventoryWithCollectors mirrors the scalar InstallPath into
	// InstallPaths for single-location collectors, while leaving collectors that
	// already populate InstallPaths (e.g. macOS PKG receipts) untouched.
	collector := &MockCollector{
		entries: map[string]*Entry{
			"single": {DisplayName: "Single", Source: "app", InstallPath: "/Applications/Single.app"},
			"multi":  {DisplayName: "Multi", Source: "pkg", InstallPath: "/usr/local", InstallPaths: []string{"/usr/local/bin", "/usr/local/lib"}},
			"none":   {DisplayName: "None", Source: "app"},
		},
	}

	inventory, _, err := GetSoftwareInventoryWithCollectors([]Collector{collector})
	assert.NoError(t, err)

	byName := make(map[string]*Entry, len(inventory))
	for _, e := range inventory {
		byName[e.DisplayName] = e
	}

	assert.Equal(t, []string{"/Applications/Single.app"}, byName["Single"].InstallPaths)
	assert.Equal(t, []string{"/usr/local/bin", "/usr/local/lib"}, byName["Multi"].InstallPaths)
	assert.Empty(t, byName["None"].InstallPaths)
}
