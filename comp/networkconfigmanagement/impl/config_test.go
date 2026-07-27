// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestConfig(t *testing.T) {
	var tests = []struct {
		name           string
		configYaml     string
		expectedConfig NcmConfig
	}{
		{
			name: "rollback enabled with all store knobs set",
			configYaml: `
network_devices:
  config_management:
    rollback:
      enabled: true
      store:
        min_configs_per_device: 3
        max_configs_per_device: 20
        max_raw_config_store_bytes: 1000000
`,
			expectedConfig: NcmConfig{
				Rollback: RollbackConfig{
					Enabled: true,
					Store: StoreConfig{
						MinConfigsPerDevice:    3,
						MaxConfigsPerDevice:    20,
						MaxRawConfigStoreBytes: 1000000,
					},
				},
			},
		},
		{
			name: "rollback enabled with store knobs partially set",
			configYaml: `
network_devices:
  config_management:
    rollback:
      enabled: true
      store:
        max_configs_per_device: 50
`,
			expectedConfig: NcmConfig{
				Rollback: RollbackConfig{
					Enabled: true,
					Store: StoreConfig{
						MaxConfigsPerDevice: 50,
					},
				},
			},
		},
		{
			name: "rollback enabled with store omitted yields zero-valued store",
			configYaml: `
network_devices:
  config_management:
    rollback:
      enabled: true
`,
			expectedConfig: NcmConfig{
				Rollback: RollbackConfig{Enabled: true},
			},
		},
		{
			name: "config_management subtree omitted entirely yields zero-valued config",
			configYaml: `
network_devices: {}
`,
			expectedConfig: NcmConfig{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := mock.NewFromYAML(t, tt.configYaml)
			testConfig, err := newConfig(mockConfig)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedConfig, *testConfig)
		})
	}
}
