// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package software

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockCollector implements Collector for testing
type MockCollector struct {
	entries  map[string]*Entry
	warnings []*Warning
	err      error
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

func TestWarnings(t *testing.T) {
	w := warnf("test %s %d", "warning", 123)
	assert.Equal(t, "test warning 123", w.Message)

	warn := Warning{Message: "test warning"}
	assert.Equal(t, "test warning", warn.Message)
}
