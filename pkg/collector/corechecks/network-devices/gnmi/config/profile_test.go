// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestLoadProfile(t *testing.T) {
	mockConfig := configmock.New(t)
	profilesRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", profilesRoot)

	profileDir := filepath.Join(profilesRoot, "gnmi.d", profilesFolder)
	require.NoError(t, writeTestProfile(profileDir, "cisco.yaml", validProfileYAML))
	require.NoError(t, writeTestProfile(profileDir, "enum.yaml", `
metrics:
  - path: /interfaces/interface/state/admin-status
    metric: snmp.ifAdminStatus
    type: gauge
    value_map:
      UP: 1
      DOWN: 2
`))

	tests := []struct {
		name        string
		profileRef  string
		wantName    string
		wantMetrics int
		errContains string
	}{
		{
			name:        "load by profile name",
			profileRef:  "cisco",
			wantName:    "cisco",
			wantMetrics: 1,
		},
		{
			name:        "load by profile name with yaml extension",
			profileRef:  "cisco.yaml",
			wantName:    "cisco",
			wantMetrics: 1,
		},
		{
			name:        "load profile with value_map",
			profileRef:  "enum",
			wantName:    "enum",
			wantMetrics: 1,
		},
		{
			name:        "missing profile",
			profileRef:  "missing",
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := LoadProfile(tt.profileRef)
			if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, profile.Name)
			assert.Len(t, profile.Metrics, tt.wantMetrics)
		})
	}
}

func TestLoadProfileAbsolutePath(t *testing.T) {
	profileDir := t.TempDir()
	profilePath := filepath.Join(profileDir, "custom.yaml")
	require.NoError(t, writeFile(profilePath, validProfileYAML))

	profile, err := LoadProfile(profilePath)
	require.NoError(t, err)
	assert.Equal(t, "custom", profile.Name)
	assert.Equal(t, profilePath, profile.Path)
}

func TestValidateProfileDefinition(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		errContains string
	}{
		{
			name: "valid profile",
			raw:  validProfileYAML,
		},
		{
			name: "empty metrics",
			raw: `
metrics: []
`,
			errContains: "at least one metric",
		},
		{
			name: "empty path",
			raw: `
metrics:
  - path: " "
    metric: snmp.ifHCInOctets
    type: monotonic_count
`,
			errContains: "`path` must not be empty",
		},
		{
			name: "empty metric name",
			raw: `
metrics:
  - path: /interfaces/interface/state/counters/in-octets
    metric: " "
    type: monotonic_count
`,
			errContains: "`metric` must not be empty",
		},
		{
			name: "unsupported metric type",
			raw: `
metrics:
  - path: /interfaces/interface/state/counters/in-octets
    metric: snmp.ifHCInOctets
    type: rate
`,
			errContains: "unsupported metric `type`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			profilesRoot := t.TempDir()
			mockConfig.SetInTest("confd_path", profilesRoot)

			profileDir := filepath.Join(profilesRoot, "gnmi.d", profilesFolder)
			require.NoError(t, writeTestProfile(profileDir, "test.yaml", tt.raw))

			_, err := LoadProfile("test")
			if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestResolveProfilePath(t *testing.T) {
	mockConfig := configmock.New(t)
	profilesRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", profilesRoot)

	profileDir := filepath.Join(profilesRoot, "gnmi.d", profilesFolder)
	require.NoError(t, writeTestProfile(profileDir, "cisco.yaml", validProfileYAML))

	got, err := resolveProfilePath("cisco")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(profileDir, "cisco.yaml"), got)
}
