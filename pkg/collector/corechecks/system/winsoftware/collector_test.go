// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winsoftware

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows/registry"
)

// MockRegistryCollector implements SoftwareCollector for testing
type MockRegistryCollector struct {
	entries map[string]*SoftwareEntry
	err     error
}

func (m *MockRegistryCollector) Collect() ([]*SoftwareEntry, []*Warning, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	var result []*SoftwareEntry
	var warnings []*Warning
	for _, entry := range m.entries {
		if entry != nil {
			result = append(result, entry)
		}
	}
	return result, warnings, nil
}

// MockMSICollector implements SoftwareCollector for testing
type MockMSICollector struct {
	entries map[string]*SoftwareEntry
	err     error
}

func (m *MockMSICollector) Collect() ([]*SoftwareEntry, []*Warning, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	var result []*SoftwareEntry
	var warnings []*Warning
	for _, entry := range m.entries {
		if entry != nil {
			result = append(result, entry)
		} else {
			warnings = append(warnings, warnf("invalid software detected"))
		}
	}
	return result, warnings, nil
}

func TestGetSoftwareInventory(t *testing.T) {
	tests := []struct {
		name              string
		registryData      map[string]*SoftwareEntry
		msiData           map[string]*SoftwareEntry
		expectedInventory []*SoftwareEntry
		expectedError     string
		expectedWarning   string
	}{
		{
			name: "System and MSI software",
			registryData: map[string]*SoftwareEntry{
				`SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\App1`: {
					DisplayName: "Test App 1",
					Version:     "1.0.0",
					InstallDate: "20240101",
					Source:      "desktop[registry]",
					Properties: map[string]string{
						"Publisher": "Test Publisher",
					},
				},
			},
			msiData: map[string]*SoftwareEntry{
				"{PRODUCT-1}": {
					DisplayName: "MSI App 1",
					Version:     "1.0.0",
					InstallDate: "20240201",
					Source:      "desktop[msi]",
					Properties: map[string]string{
						"Publisher": "MSI Publisher",
					},
				},
			},
			expectedInventory: []*SoftwareEntry{
				{
					DisplayName: "Test App 1",
					Version:     "1.0.0",
					InstallDate: "20240101",
					Source:      "desktop[registry]",
					Properties: map[string]string{
						"Publisher": "Test Publisher",
					},
				},
				{
					DisplayName: "MSI App 1",
					Version:     "1.0.0",
					InstallDate: "20240201",
					Source:      "desktop[msi]",
					Properties: map[string]string{
						"Publisher": "MSI Publisher",
					},
				},
			},
		},
		{
			name: "User hives - loaded and mounted",
			registryData: map[string]*SoftwareEntry{
				"LoadedUserApp": {
					DisplayName: "Loaded User App",
					Version:     "1.0.0",
					InstallDate: "20240301",
					UserSID:     "S-1-5-21-123456789-0",
					Source:      "desktop[registry]",
					Properties: map[string]string{
						"Publisher": "User Publisher",
					},
				},
				"MountedUserApp": {
					DisplayName: "Mounted User App",
					Version:     "1.0.0",
					InstallDate: "20240301",
					UserSID:     "S-1-5-21-123456789-1",
					Source:      "desktop[registry]",
					Properties: map[string]string{
						"Publisher": "User Publisher",
					},
				},
				"SystemApp": {
					DisplayName: "System App",
					Version:     "1.0.0",
					InstallDate: "20240302",
					Source:      "desktop[registry]",
					Properties: map[string]string{
						"Publisher": "System Publisher",
					},
				},
			},
			msiData: nil,
			expectedInventory: []*SoftwareEntry{
				{
					DisplayName: "Loaded User App",
					Version:     "1.0.0",
					InstallDate: "20240301",
					UserSID:     "S-1-5-21-123456789-0",
					Source:      "desktop[registry]",
					Properties: map[string]string{
						"Publisher": "User Publisher",
					},
				},
				{
					DisplayName: "Mounted User App",
					Version:     "1.0.0",
					InstallDate: "20240301",
					UserSID:     "S-1-5-21-123456789-1",
					Source:      "desktop[registry]",
					Properties: map[string]string{
						"Publisher": "User Publisher",
					},
				},
				{
					DisplayName: "System App",
					Version:     "1.0.0",
					InstallDate: "20240302",
					Source:      "desktop[registry]",
					Properties: map[string]string{
						"Publisher": "System Publisher",
					},
				},
			},
		},
		{
			name:         "Invalid MSI product",
			registryData: map[string]*SoftwareEntry{},
			msiData: map[string]*SoftwareEntry{
				"{INVALID-PRODUCT}": nil,
			},
			expectedInventory: []*SoftwareEntry{},
			expectedWarning:   "invalid software detected",
		},
		{
			name: "Unicode software names",
			registryData: map[string]*SoftwareEntry{
				`SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{DEEFE46F-60F2-430B-AE0A-15A76E57B767}`: {
					DisplayName: "Contrôle d'intégrité du PC Windows",
					Version:     "3.9.2402.14001",
					InstallDate: "20240301",
					Source:      "desktop[registry]",
					Properties: map[string]string{
						"Publisher": "Microsoft Corporation",
					},
				},
			},
			msiData: nil,
			expectedInventory: []*SoftwareEntry{
				{
					DisplayName: "Contrôle d'intégrité du PC Windows",
					Version:     "3.9.2402.14001",
					InstallDate: "20240301",
					Source:      "desktop[registry]",
					Properties: map[string]string{
						"Publisher": "Microsoft Corporation",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock collectors
			mockRegistryCollector := &MockRegistryCollector{
				entries: tt.registryData,
			}
			mockMSICollector := &MockMSICollector{
				entries: tt.msiData,
			}

			// Test GetSoftwareInventory with mock collectors
			inventory, warn, err := GetSoftwareInventoryWithCollectors([]SoftwareCollector{
				mockRegistryCollector,
				mockMSICollector,
			})

			// Verify results
			if tt.expectedError != "" {
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedInventory, inventory)
			}

			if tt.expectedWarning != "" {
				assert.NotEmpty(t, warn)
				found := false
				for _, w := range warn {
					if w != nil && strings.Contains(w.Message, tt.expectedWarning) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected warning '%s' not found in warnings", tt.expectedWarning)
			} else {
				assert.Empty(t, warn)
			}
		})
	}
}

func TestWarningWithInvalidMSIProduct(t *testing.T) {
	// Mock collectors
	mockRegistryCollector := &MockRegistryCollector{
		entries: map[string]*SoftwareEntry{},
	}
	mockMSICollector := &MockMSICollector{
		entries: map[string]*SoftwareEntry{
			"{INVALID-PRODUCT}": nil,
		},
	}

	// Test GetSoftwareInventory with mock collectors
	_, warn, err := GetSoftwareInventoryWithCollectors([]SoftwareCollector{
		mockRegistryCollector,
		mockMSICollector,
	})

	assert.NoError(t, err)
	assert.NotEmpty(t, warn)
	assert.Contains(t, warn[0].Message, "invalid software detected")
}

func TestRegistryError(t *testing.T) {
	// Mock collectors with error
	mockRegistryCollector := &MockRegistryCollector{
		err: fmt.Errorf("registry error"),
	}
	mockMSICollector := &MockMSICollector{
		entries: map[string]*SoftwareEntry{},
	}

	// Test GetSoftwareInventory with mock collectors
	inventory, warn, err := GetSoftwareInventoryWithCollectors([]SoftwareCollector{
		mockRegistryCollector,
		mockMSICollector,
	})

	assert.NoError(t, err) // Should not error, just warn
	assert.NotEmpty(t, warn)
	assert.Contains(t, warn[0].Message, "error collecting software")
	assert.Empty(t, inventory)
}

func TestMSIDatabaseError(t *testing.T) {
	// Mock collectors with MSI error
	mockRegistryCollector := &MockRegistryCollector{
		entries: map[string]*SoftwareEntry{},
	}
	mockMSICollector := &MockMSICollector{
		err: fmt.Errorf("MSI error"),
	}

	// Test GetSoftwareInventory with mock collectors
	inventory, warn, err := GetSoftwareInventoryWithCollectors([]SoftwareCollector{
		mockRegistryCollector,
		mockMSICollector,
	})

	assert.NoError(t, err) // Should not error, just warn
	assert.NotEmpty(t, warn)
	assert.Contains(t, warn[0].Message, "error collecting software")
	assert.Empty(t, inventory)
}

func TestWarning_Message(t *testing.T) {
	w := Warning{Message: "test warning"}
	assert.Equal(t, "test warning", w.Message)
}

func TestWarnf(t *testing.T) {
	w := warnf("test %s", "warning")
	assert.Equal(t, "test warning", w.Message)
}

func TestMountUnmountHive(t *testing.T) {
	// Test mountHive with non-existent path
	err := mountHive("nonexistent/path/NTUSER.DAT")
	assert.Error(t, err)

	// Test unmountHive when no hive is mounted
	err = unmountHive()
	assert.Error(t, err)
}

func deleteRegistryKeyRecursive(t *testing.T, root registry.Key, path string) error {
	// Open the key
	key, err := registry.OpenKey(root, path, registry.ALL_ACCESS)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to open key: %v", err)
	}
	defer func() { _ = key.Close() }()

	// List subkeys
	subkeys, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return fmt.Errorf("failed to list subkeys: %v", err)
	}

	// Delete each subkey
	for _, subkey := range subkeys {
		fullPath := path + "\\" + subkey
		err = deleteRegistryKeyRecursive(t, root, fullPath)
		if err != nil {
			return err
		}
	}

	// Close the key before trying to delete it
	_ = key.Close()

	// Delete the key itself
	err = registry.DeleteKey(root, path)
	if err != nil {
		return fmt.Errorf("failed to delete key: %v", err)
	}

	return nil
}

func TestCollectFromKey(t *testing.T) {
	// Create a test key in HKCU (safer than HKLM which requires admin rights)
	testRoot := "SOFTWARE\\DatadogTest\\WinSoftware"
	reg := registry.CURRENT_USER

	// Clean up any leftover test keys recursively
	err := deleteRegistryKeyRecursive(t, reg, testRoot)
	if err != nil {
		t.Logf("Initial cleanup error: %v", err)
	}

	// Ensure cleanup after test
	defer func() {
		err := deleteRegistryKeyRecursive(t, reg, testRoot)
		if err != nil {
			t.Logf("Final cleanup error: %v", err)
		}
	}()

	// Create test keys
	key, _, err := registry.CreateKey(reg, testRoot, registry.ALL_ACCESS)
	if err != nil {
		t.Fatalf("Failed to create test registry key: %v", err)
	}

	// Test data with Unicode characters
	testData := []struct {
		subKey      string
		displayName string
		version     string
		publisher   string
	}{
		{
			subKey:      "{DEEFE46F-60F2-430B-AE0A-15A76E57B767}",
			displayName: "Contrôle d'intégrité du PC Windows",
			version:     "3.9.2402.14001",
			publisher:   "Microsoft Corporation",
		},
		{
			subKey:      "{TEST-UNICODE-2}",
			displayName: "プログラムと機能", // "Programs and Features" in Japanese
			version:     "1.0.0",
			publisher:   "テスト発行者", // "Test Publisher" in Japanese
		},
		{
			subKey:      "{TEST-UNICODE-3}",
			displayName: "Інсталятор Windows", // "Windows Installer" in Ukrainian
			version:     "2.0.0",
			publisher:   "тестовий видавець", // "Test Publisher" in Ukrainian
		},
	}

	// Create test subkeys with Unicode data
	var subKeys []registry.Key
	for _, td := range testData {
		subKey, _, err := registry.CreateKey(key, td.subKey, registry.ALL_ACCESS)
		if err != nil {
			t.Fatalf("Failed to create test subkey: %v", err)
		}
		subKeys = append(subKeys, subKey)

		err = subKey.SetStringValue("DisplayName", td.displayName)
		if err != nil {
			t.Fatalf("Failed to set DisplayName: %v", err)
		}

		err = subKey.SetStringValue("DisplayVersion", td.version)
		if err != nil {
			t.Fatalf("Failed to set DisplayVersion: %v", err)
		}

		err = subKey.SetStringValue("Publisher", td.publisher)
		if err != nil {
			t.Fatalf("Failed to set Publisher: %v", err)
		}
	}

	// Close all keys before testing
	for _, sk := range subKeys {
		_ = sk.Close()
	}
	_ = key.Close()

	// Test CollectFromKey
	results, warnings := collectFromKey(reg, testRoot, registry.WOW64_64KEY)
	assert.Empty(t, warnings, "Expected no warnings")

	// Verify results
	assert.Equal(t, len(testData), len(results), "Should find all test entries")

	for _, td := range testData {
		found := false
		for _, result := range results {
			if result.Properties[msiProductCode] == td.subKey {
				found = true
				assert.Equal(t, td.displayName, result.DisplayName, "Unicode DisplayName should match for %s", td.subKey)
				assert.Equal(t, td.version, result.Version, "Version should match for %s", td.subKey)
				assert.Equal(t, td.publisher, result.Properties["Publisher"], "Unicode Publisher should match for %s", td.subKey)
				assert.Equal(t, "desktop[registry]", result.Source, "Source should be set for %s", td.subKey)
				break
			}
		}
		assert.True(t, found, "Should find test entry %s", td.subKey)
	}
}
