// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package workloadselectionimpl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

// TestExtractPolicyID tests policy ID extraction from config paths
func TestExtractPolicyID(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "valid path format",
			path:     "datadog/123/product/my-policy-id/hash",
			expected: "my-policy-id",
		},
		{
			name:     "valid path with numeric prefix in policy ID",
			path:     "datadog/123/product/5.my-policy/hash",
			expected: "5.my-policy",
		},
		{
			name:     "invalid path format",
			path:     "invalid/path",
			expected: "",
		},
		{
			name:     "path missing policy ID section",
			path:     "datadog/123/product/",
			expected: "",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "path with special characters in policy ID",
			path:     "datadog/456/apm-policies/policy_name-123/abcdef",
			expected: "policy_name-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPolicyID(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractOrderFromPolicyID tests order extraction from policy IDs
func TestExtractOrderFromPolicyID(t *testing.T) {
	tests := []struct {
		name     string
		policyID string
		expected int
	}{
		{
			name:     "policy ID with numeric prefix",
			policyID: "5.my-policy",
			expected: 5,
		},
		{
			name:     "policy ID without numeric prefix",
			policyID: "my-policy",
			expected: 0,
		},
		{
			name:     "policy ID with only number (no dot)",
			policyID: "42",
			expected: 0,
		},
		{
			name:     "policy ID with multiple dots",
			policyID: "5.10.policy",
			expected: 5,
		},
		{
			name:     "policy ID with non-numeric prefix",
			policyID: "abc.policy",
			expected: 0,
		},
		{
			name:     "empty policy ID",
			policyID: "",
			expected: 0,
		},
		{
			name:     "policy ID with large number",
			policyID: "100.high-priority",
			expected: 100,
		},
		{
			name:     "policy ID with zero prefix",
			policyID: "0.default",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractOrderFromPolicyID(tt.policyID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMergeConfigs tests the merging of multiple policy configs
func TestMergeConfigs(t *testing.T) {
	tests := []struct {
		name           string
		configs        []policyConfig
		expectedResult string
		expectError    bool
	}{
		{
			name: "single config",
			configs: []policyConfig{
				{
					path:   "path1",
					order:  0,
					config: []byte(`{"policies":[{"rule":"allow"}]}`),
				},
			},
			expectedResult: `{"policies":[{"rule":"allow"}]}`,
			expectError:    false,
		},
		{
			name: "multiple configs with different policies",
			configs: []policyConfig{
				{
					path:   "path1",
					order:  0,
					config: []byte(`{"policies":[{"rule":"allow"}]}`),
				},
				{
					path:   "path2",
					order:  1,
					config: []byte(`{"policies":[{"rule":"deny"}]}`),
				},
			},
			expectedResult: `{"policies":[{"rule":"allow"},{"rule":"deny"}]}`,
			expectError:    false,
		},
		{
			name:           "empty config list",
			configs:        []policyConfig{},
			expectedResult: `{"policies":[]}`,
			expectError:    false,
		},
		{
			name: "config with empty policies array",
			configs: []policyConfig{
				{
					path:   "path1",
					order:  0,
					config: []byte(`{"policies":[]}`),
				},
			},
			expectedResult: `{"policies":[]}`,
			expectError:    false,
		},
		{
			name: "invalid JSON in one config",
			configs: []policyConfig{
				{
					path:   "path1",
					order:  0,
					config: []byte(`{"policies":[{"rule":"allow"}]}`),
				},
				{
					path:   "path2",
					order:  1,
					config: []byte(`invalid json`),
				},
			},
			expectError: true,
		},
		{
			name: "multiple configs with nested complex policies",
			configs: []policyConfig{
				{
					path:   "path1",
					order:  0,
					config: []byte(`{"policies":[{"name":"policy1","rules":[{"action":"allow","service":"*"}]}]}`),
				},
				{
					path:   "path2",
					order:  1,
					config: []byte(`{"policies":[{"name":"policy2","rules":[{"action":"deny","service":"test"}]}]}`),
				},
			},
			expectedResult: `{"policies":[{"name":"policy1","rules":[{"action":"allow","service":"*"}]},{"name":"policy2","rules":[{"action":"deny","service":"test"}]}]}`,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := mergeConfigs(tt.configs)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				// Normalize JSON for comparison
				var expected, actual interface{}
				require.NoError(t, json.Unmarshal([]byte(tt.expectedResult), &expected))
				require.NoError(t, json.Unmarshal(result, &actual))
				assert.Equal(t, expected, actual)
			}
		})
	}
}

// TestRemoveConfig tests config file removal
func TestRemoveConfig(t *testing.T) {
	tests := []struct {
		name        string
		setupFiles  func(t *testing.T, tempDir string)
		expectError bool
	}{
		{
			name: "config file exists",
			setupFiles: func(t *testing.T, tempDir string) {
				configPath = filepath.Join(tempDir, "compiled.bin")
				require.NoError(t, os.WriteFile(configPath, []byte("test"), 0644))
			},
			expectError: false,
		},
		{
			name: "config file doesn't exist",
			setupFiles: func(_ *testing.T, tempDir string) {
				configPath = filepath.Join(tempDir, "compiled.bin")
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tt.setupFiles(t, tempDir)

			mockConfig := config.NewMock(t)
			mockLog := logmock.New(t)

			component := &workloadselectionComponent{
				log:    mockLog,
				config: mockConfig,
			}

			err := component.removeConfig()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify file is removed
				_, err := os.Stat(configPath)
				assert.True(t, os.IsNotExist(err))
			}
		})
	}
}
