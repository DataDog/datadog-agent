// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestParseInstanceConfig(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		want        InstanceConfig
		errContains string
	}{
		{
			name: "valid config with defaults",
			raw: `
address: 203.0.113.10
username: admin
password: secret
profile: cisco
`,
			want: InstanceConfig{
				Address:               "203.0.113.10",
				Port:                  DefaultPort,
				Username:              "admin",
				Password:              "secret",
				Profile:               "cisco",
				MinCollectionInterval: DefaultMinCollectionInterval,
				CollectTopology:       false,
			},
		},
		{
			name: "valid config with overrides",
			raw: `
address: 203.0.113.11
port: 9339
username: ops
password: s3cret
profile: juniper
min_collection_interval: 30
tags:
  - env:prod
collect_topology: true
`,
			want: InstanceConfig{
				Address:               "203.0.113.11",
				Port:                  9339,
				Username:              "ops",
				Password:              "s3cret",
				Profile:               "juniper",
				MinCollectionInterval: 30,
				Tags:                  []string{"env:prod"},
				CollectTopology:       true,
			},
		},
		{
			name: "missing address",
			raw: `
username: admin
password: secret
profile: cisco
`,
			errContains: "`address` is required",
		},
		{
			name: "missing username",
			raw: `
address: 203.0.113.10
password: secret
profile: cisco
`,
			errContains: "`username` is required",
		},
		{
			name: "missing password",
			raw: `
address: 203.0.113.10
username: admin
profile: cisco
`,
			errContains: "`password` is required",
		},
		{
			name: "missing profile",
			raw: `
address: 203.0.113.10
username: admin
password: secret
`,
			errContains: "`profile` is required",
		},
		{
			name: "invalid port",
			raw: `
address: 203.0.113.10
port: 0
username: admin
password: secret
profile: cisco
`,
			errContains: "invalid `port`",
		},
		{
			name: "invalid min_collection_interval",
			raw: `
address: 203.0.113.10
username: admin
password: secret
profile: cisco
min_collection_interval: 0
`,
			errContains: "invalid `min_collection_interval`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInstanceConfig(integration.Data([]byte(tt.raw)))
			if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.NotContains(t, err.Error(), "secret")
				assert.NotContains(t, err.Error(), "s3cret")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, *got)
		})
	}
}

func TestCheckConfigStringRedactsPassword(t *testing.T) {
	cfg := CheckConfig{
		Instance: InstanceConfig{
			Address:  "203.0.113.10",
			Port:     DefaultPort,
			Username: "admin",
			Password: "super-secret-password",
			Profile:  "cisco",
		},
		Profile: ProfileDefinition{
			Metrics: []MetricConfig{
				{
					Path:   "/interfaces/interface/state/counters/in-octets",
					Metric: "snmp.ifHCInOctets",
					Type:   MetricTypeMonotonicCount,
				},
			},
		},
	}

	str := cfg.String()
	assert.NotContains(t, str, "super-secret-password")
	assert.Contains(t, str, "203.0.113.10")
	assert.Contains(t, str, "admin")
	assert.Contains(t, str, "cisco")
}

func TestInstanceConfigStringRedactsPassword(t *testing.T) {
	instance := InstanceConfig{
		Address:  "203.0.113.10",
		Username: "admin",
		Password: "super-secret-password",
		Profile:  "cisco",
	}

	str := instance.String()
	assert.NotContains(t, str, "super-secret-password")
	assert.Contains(t, str, "admin")
}

func TestNewCheckConfig(t *testing.T) {
	mockConfig := configmock.New(t)
	profilesRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", profilesRoot)

	profileDir := filepath.Join(profilesRoot, "gnmi.d", profilesFolder)
	require.NoError(t, writeTestProfile(profileDir, "cisco.yaml", validProfileYAML))

	rawInstance := []byte(`
address: 203.0.113.10
username: admin
password: secret
profile: cisco
`)

	cfg, err := NewCheckConfig(integration.Data(rawInstance))
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "cisco", cfg.Profile.Name)
	assert.Len(t, cfg.Profile.Metrics, 1)
	assert.Equal(t, MetricTypeMonotonicCount, cfg.Profile.Metrics[0].Type)
}

const validProfileYAML = `
metrics:
  - path: /interfaces/interface/state/counters/in-octets
    metric: snmp.ifHCInOctets
    type: monotonic_count
    tags:
      interface: name
`

func writeTestProfile(dir, name, content string) error {
	if err := mkdirAll(dir); err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, name), content)
}
