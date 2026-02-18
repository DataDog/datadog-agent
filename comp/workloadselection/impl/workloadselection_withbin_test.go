// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || windows

package workloadselectionimpl

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// getBinaryPath returns the correct binary path based on the platform
func getBinaryPath(tempDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(tempDir, "bin", "dd-compile-policy.exe")
	}
	return filepath.Join(tempDir, "embedded", "bin", "dd-compile-policy")
}

// TestNewComponent tests the component initialization
func TestNewComponent(t *testing.T) {
	tests := []struct {
		name                     string
		workloadSelectionEnabled bool
		setupBinary              bool
		expectRCListener         bool
	}{
		{
			name:                     "workload selection enabled and binary available",
			workloadSelectionEnabled: true,
			setupBinary:              true,
			expectRCListener:         true,
		},
		{
			name:                     "workload selection disabled",
			workloadSelectionEnabled: false,
			setupBinary:              true,
			expectRCListener:         false,
		},
		{
			name:                     "workload selection enabled but binary missing",
			workloadSelectionEnabled: true,
			setupBinary:              false,
			expectRCListener:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup temp directory for test
			tempDir := t.TempDir()

			// Override getInstallPath for testing
			originalGetInstallPath := getInstallPath
			getInstallPath = func() string { return tempDir }
			t.Cleanup(func() {
				getInstallPath = originalGetInstallPath
			})

			// Create binary if needed
			if tt.setupBinary {
				binaryPath := getBinaryPath(tempDir)
				require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0755))
				// Create executable file
				require.NoError(t, os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755))
			}

			// Create mock components
			mockConfig := config.NewMock(t)
			mockConfig.SetWithoutSource("apm_config.workload_selection", tt.workloadSelectionEnabled)
			mockLog := logmock.New(t)

			reqs := Requires{
				Log:    mockLog,
				Config: mockConfig,
			}

			// Create component
			provides, err := NewComponent(reqs)
			require.NoError(t, err)
			assert.NotNil(t, provides.Comp)

			// Check if RCListener was created
			hasListener := len(provides.RCListener.ListenerProvider) > 0
			assert.Equal(t, tt.expectRCListener, hasListener)
		})
	}
}

// TestIsCompilePolicyBinaryAvailable tests the binary availability check
func TestIsCompilePolicyBinaryAvailable(t *testing.T) {
	tests := []struct {
		name         string
		setupFunc    func(t *testing.T, tempDir string) string
		expectResult bool
	}{
		{
			name: "binary exists and is executable",
			setupFunc: func(t *testing.T, tempDir string) string {
				binaryPath := getBinaryPath(tempDir)
				require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0755))
				require.NoError(t, os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755))
				return tempDir
			},
			expectResult: true,
		},
		{
			name: "binary doesn't exist",
			setupFunc: func(_ *testing.T, tempDir string) string {
				return tempDir
			},
			expectResult: false,
		},
		{
			name: "binary exists but is not executable",
			setupFunc: func(t *testing.T, tempDir string) string {
				binaryPath := getBinaryPath(tempDir)
				require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0755))
				require.NoError(t, os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0644))
				return tempDir
			},
			// On Windows, executability is determined by extension, not permissions
			// So this test expects different results on Windows vs Unix
			expectResult: runtime.GOOS == "windows",
		},
		{
			name: "binary exists but is a directory",
			setupFunc: func(t *testing.T, tempDir string) string {
				binaryPath := getBinaryPath(tempDir)
				require.NoError(t, os.MkdirAll(binaryPath, 0755))
				return tempDir
			},
			expectResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			installPath := tt.setupFunc(t, tempDir)

			// Override getInstallPath for testing
			originalGetInstallPath := getInstallPath
			getInstallPath = func() string { return installPath }
			t.Cleanup(func() {
				getInstallPath = originalGetInstallPath
			})

			mockConfig := config.NewMock(t)
			mockLog := logmock.New(t)

			component := &workloadselectionComponent{
				log:    mockLog,
				config: mockConfig,
			}

			result := component.isCompilePolicyBinaryAvailable()
			assert.Equal(t, tt.expectResult, result)
		})
	}
}

// TestOnConfigUpdate_NoConfigs tests config update with empty map
func TestOnConfigUpdate_NoConfigs(t *testing.T) {
	tempDir := t.TempDir()

	// Override getInstallPath for testing
	originalGetInstallPath := getInstallPath
	getInstallPath = func() string { return tempDir }
	t.Cleanup(func() {
		getInstallPath = originalGetInstallPath
	})

	// Create mock components
	mockConfig := config.NewMock(t)
	mockLog := logmock.New(t)

	component := &workloadselectionComponent{
		log:    mockLog,
		config: mockConfig,
	}

	// Create config files to be removed
	configPath = filepath.Join(tempDir, "test-compiled.bin")
	require.NoError(t, os.WriteFile(configPath, []byte("test"), 0644))

	// Call with empty updates
	callbackCalled := false
	component.onConfigUpdate(map[string]state.RawConfig{}, func(_ string, _ state.ApplyStatus) {
		callbackCalled = true
	})

	// Verify file is removed
	_, err := os.Stat(configPath)
	assert.True(t, os.IsNotExist(err))

	// Callback should not be called for empty updates
	assert.False(t, callbackCalled)
}

// TestOnConfigUpdate_SingleConfig tests config update with a single valid config
func TestOnConfigUpdate_SingleConfig(t *testing.T) {
	tempDir := t.TempDir()

	// Override getInstallPath for testing
	originalGetInstallPath := getInstallPath
	getInstallPath = func() string { return tempDir }
	t.Cleanup(func() {
		getInstallPath = originalGetInstallPath
	})

	// Create mock components
	mockConfig := config.NewMock(t)
	mockLog := logmock.New(t)

	component := &workloadselectionComponent{
		log:    mockLog,
		config: mockConfig,
	}

	configPath = filepath.Join(tempDir, "test-compiled.bin")

	updates := map[string]state.RawConfig{
		"datadog/123/apm-policies/policy1/hash": {
			Config: []byte(`{"policies":[{"rule":"allow"}]}`),
		},
	}

	var callbackPath string
	var callbackStatus state.ApplyStatus
	component.onConfigUpdate(updates, func(path string, status state.ApplyStatus) {
		callbackPath = path
		callbackStatus = status
	})

	// Note: Without actual binary compilation, this will fail during compile step
	// But we can verify the callback was called with error
	assert.Equal(t, "datadog/123/apm-policies/policy1/hash", callbackPath)
	assert.Equal(t, state.ApplyStateError, callbackStatus.State)
}

// TestOnConfigUpdate_MultipleConfigs tests config update with multiple configs and ordering
func TestOnConfigUpdate_MultipleConfigs(t *testing.T) {
	tempDir := t.TempDir()

	// Override getInstallPath for testing
	originalGetInstallPath := getInstallPath
	getInstallPath = func() string { return tempDir }
	t.Cleanup(func() {
		getInstallPath = originalGetInstallPath
	})

	mockConfig := config.NewMock(t)
	mockLog := logmock.New(t)

	component := &workloadselectionComponent{
		log:    mockLog,
		config: mockConfig,
	}

	configPath = filepath.Join(tempDir, "test-compiled.bin")

	tests := []struct {
		name          string
		updates       map[string]state.RawConfig
		expectedOrder []string // Expected order of policy IDs
	}{
		{
			name: "configs without ordering (sorted alphabetically)",
			updates: map[string]state.RawConfig{
				"datadog/123/apm-policies/zebra/hash": {
					Config: []byte(`{"policies":[{"name":"zebra"}]}`),
				},
				"datadog/123/apm-policies/alpha/hash": {
					Config: []byte(`{"policies":[{"name":"alpha"}]}`),
				},
			},
			expectedOrder: []string{"alpha", "zebra"},
		},
		{
			name: "configs with different orders",
			updates: map[string]state.RawConfig{
				"datadog/123/apm-policies/5.high/hash": {
					Config: []byte(`{"policies":[{"name":"high"}]}`),
				},
				"datadog/123/apm-policies/1.low/hash": {
					Config: []byte(`{"policies":[{"name":"low"}]}`),
				},
				"datadog/123/apm-policies/10.highest/hash": {
					Config: []byte(`{"policies":[{"name":"highest"}]}`),
				},
			},
			expectedOrder: []string{"1.low", "5.high", "10.highest"},
		},
		{
			name: "mix of ordered and unordered configs",
			updates: map[string]state.RawConfig{
				"datadog/123/apm-policies/5.ordered/hash": {
					Config: []byte(`{"policies":[{"name":"ordered"}]}`),
				},
				"datadog/123/apm-policies/unordered/hash": {
					Config: []byte(`{"policies":[{"name":"unordered"}]}`),
				},
			},
			expectedOrder: []string{"unordered", "5.ordered"}, // order=0 comes before order=5
		},
		{
			name: "configs with same order (secondary sort by path)",
			updates: map[string]state.RawConfig{
				"datadog/123/apm-policies/5.zebra/hash": {
					Config: []byte(`{"policies":[{"name":"zebra"}]}`),
				},
				"datadog/123/apm-policies/5.alpha/hash": {
					Config: []byte(`{"policies":[{"name":"alpha"}]}`),
				},
			},
			expectedOrder: []string{"5.alpha", "5.zebra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callbackPaths := []string{}
			component.onConfigUpdate(tt.updates, func(path string, _ state.ApplyStatus) {
				callbackPaths = append(callbackPaths, path)
			})

			// Extract policy IDs from callback paths and verify order
			var actualPolicyIDs []string
			for _, path := range callbackPaths {
				policyID := extractPolicyID(path)
				if policyID != "" {
					actualPolicyIDs = append(actualPolicyIDs, policyID)
				}
			}

			assert.Equal(t, tt.expectedOrder, actualPolicyIDs)
		})
	}
}

// TestOnConfigUpdate_ErrorHandling tests error handling during config updates
func TestOnConfigUpdate_ErrorHandling(t *testing.T) {
	tempDir := t.TempDir()

	// Override getInstallPath for testing
	originalGetInstallPath := getInstallPath
	getInstallPath = func() string { return tempDir }
	t.Cleanup(func() {
		getInstallPath = originalGetInstallPath
	})

	mockConfig := config.NewMock(t)
	mockLog := logmock.New(t)

	component := &workloadselectionComponent{
		log:    mockLog,
		config: mockConfig,
	}

	configPath = filepath.Join(tempDir, "test-compiled.bin")

	tests := []struct {
		name        string
		updates     map[string]state.RawConfig
		expectError bool
	}{
		{
			name: "merge fails with invalid JSON",
			updates: map[string]state.RawConfig{
				"datadog/123/apm-policies/policy1/hash": {
					Config: []byte(`invalid json`),
				},
			},
			expectError: true,
		},
		{
			name: "one invalid config in batch",
			updates: map[string]state.RawConfig{
				"datadog/123/apm-policies/valid/hash": {
					Config: []byte(`{"policies":[{"name":"valid"}]}`),
				},
				"datadog/123/apm-policies/invalid/hash": {
					Config: []byte(`{invalid}`),
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorStates := make(map[string]state.ApplyStatus)
			component.onConfigUpdate(tt.updates, func(path string, status state.ApplyStatus) {
				errorStates[path] = status
			})

			if tt.expectError {
				// Verify all configs received error callbacks
				for path := range tt.updates {
					status, exists := errorStates[path]
					assert.True(t, exists)
					assert.Equal(t, state.ApplyStateError, status.State)
					assert.NotEmpty(t, status.Error)
				}
			}
		})
	}
}
