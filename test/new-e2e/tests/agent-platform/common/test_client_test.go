// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// mockFileManager implements filemanager.FileManager for testing
type mockFileManager struct {
	files map[string][]byte
}

func newMockFileManager() *mockFileManager {
	return &mockFileManager{
		files: make(map[string][]byte),
	}
}

func (m *mockFileManager) ReadFile(path string) ([]byte, error) {
	if content, ok := m.files[path]; ok {
		return content, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFileManager) WriteFile(path string, content []byte) (int64, error) {
	m.files[path] = content
	return int64(len(content)), nil
}

func (m *mockFileManager) ReadDir(_ string) ([]os.DirEntry, error) {
	return nil, nil
}

func (m *mockFileManager) FileExists(path string) (bool, error) {
	_, ok := m.files[path]
	return ok, nil
}

// normalizeMapForTest converts all map types to map[string]any recursively for consistent comparison
func normalizeMapForTest(input any) any {
	switch v := input.(type) {
	case map[interface{}]any:
		result := make(map[string]any)
		for key, value := range v {
			if strKey, ok := key.(string); ok {
				result[strKey] = normalizeMapForTest(value)
			}
		}
		return result
	case map[string]any:
		result := make(map[string]any)
		for key, value := range v {
			result[key] = normalizeMapForTest(value)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = normalizeMapForTest(item)
		}
		return result
	default:
		return v
	}
}

func TestSetConfig(t *testing.T) {
	tests := []struct {
		name           string
		existingConfig string
		key            string
		value          string
		expectedYAML   map[string]any
	}{
		{
			name:           "single level key",
			existingConfig: "",
			key:            "api_key",
			value:          "test123",
			expectedYAML: map[string]any{
				"api_key": "test123",
			},
		},
		{
			name:           "two level nested key",
			existingConfig: "",
			key:            "logs_config.enabled",
			value:          "true",
			expectedYAML: map[string]any{
				"logs_config": map[string]any{
					"enabled": "true",
				},
			},
		},
		{
			name:           "three level nested key",
			existingConfig: "",
			key:            "apm_config.obfuscation.elasticsearch.enabled",
			value:          "true",
			expectedYAML: map[string]any{
				"apm_config": map[string]any{
					"obfuscation": map[string]any{
						"elasticsearch": map[string]any{
							"enabled": "true",
						},
					},
				},
			},
		},
		{
			name:           "five level deeply nested key",
			existingConfig: "",
			key:            "a.b.c.d.e",
			value:          "deep_value",
			expectedYAML: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": map[string]any{
							"d": map[string]any{
								"e": "deep_value",
							},
						},
					},
				},
			},
		},
		{
			name: "add to existing config",
			existingConfig: `api_key: existing123
logs_config:
  enabled: false
`,
			key:   "logs_config.log_level",
			value: "debug",
			expectedYAML: map[string]any{
				"api_key": "existing123",
				"logs_config": map[string]any{
					"enabled":   false,
					"log_level": "debug",
				},
			},
		},
		{
			name: "update existing nested value",
			existingConfig: `apm_config:
  enabled: false
  receiver_port: 8126
`,
			key:   "apm_config.enabled",
			value: "true",
			expectedYAML: map[string]any{
				"apm_config": map[string]any{
					"enabled":       "true",
					"receiver_port": 8126,
				},
			},
		},
		{
			name: "add deeply nested key to existing config",
			existingConfig: `api_key: test
apm_config:
  enabled: true
`,
			key:   "apm_config.obfuscation.elasticsearch.enabled",
			value: "true",
			expectedYAML: map[string]any{
				"api_key": "test",
				"apm_config": map[string]any{
					"enabled": true,
					"obfuscation": map[string]any{
						"elasticsearch": map[string]any{
							"enabled": "true",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			fm := newMockFileManager()
			confPath := filepath.Join("/tmp", "test.yaml")

			// Add existing config if provided
			if tt.existingConfig != "" {
				fm.files[confPath] = []byte(tt.existingConfig)
			}

			client := &TestClient{
				FileManager: fm,
			}

			// Execute
			err := client.SetConfig(confPath, tt.key, tt.value)
			require.NoError(t, err)

			// Verify
			content, ok := fm.files[confPath]
			require.True(t, ok, "config file should exist")

			var actualYAML map[string]any
			err = yaml.Unmarshal(content, &actualYAML)
			require.NoError(t, err)

			// Normalize both maps to handle yaml.v2's map[interface{}]any conversion
			normalizedActual := normalizeMapForTest(actualYAML)
			normalizedExpected := normalizeMapForTest(tt.expectedYAML)

			assert.Equal(t, normalizedExpected, normalizedActual)
		})
	}
}
