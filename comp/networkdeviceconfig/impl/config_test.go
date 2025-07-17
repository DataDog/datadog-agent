// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkdeviceconfigimpl

import (
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConfig(t *testing.T) {
	var tests = []struct {
		name           string
		configYaml     string
		expectedConfig ProcessedNcmConfig
	}{
		{
			name: "NCM configured with one device using SSH configurations",
			configYaml: `
network_device_config_management:
  namespace: test
  devices:
    - ip_address: 10.0.0.1
      auth:
        username: admin
        password: password
        port: 22
        protocol: tcp
`,
			expectedConfig: ProcessedNcmConfig{
				Namespace: "test",
				Devices: map[string]DeviceConfig{
					"10.0.0.1": {
						IPAddress: "10.0.0.1",
						Auth: AuthCredentials{
							Username: "admin",
							Password: "password",
							Port:     "22",
							Protocol: "tcp",
						},
					},
				},
			},
		},
		{
			name: "NCM configured with multiple devices using SSH",
			configYaml: `
network_device_config_management:
  namespace: test
  devices:
    - ip_address: 10.0.0.1
      auth:
        username: admin
        password: password
        port: 22
        protocol: tcp
    - ip_address: 10.0.0.2
      auth:
        username: user
        password: pass
        port: 22
        protocol: tcp
`,
			expectedConfig: ProcessedNcmConfig{
				Namespace: "test",
				Devices: map[string]DeviceConfig{
					"10.0.0.1": {
						IPAddress: "10.0.0.1",
						Auth: AuthCredentials{
							Username: "admin",
							Password: "password",
							Port:     "22",
							Protocol: "tcp",
						},
					},
					"10.0.0.2": {
						IPAddress: "10.0.0.2",
						Auth: AuthCredentials{
							Username: "user",
							Password: "pass",
							Port:     "22",
							Protocol: "tcp",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := mock.NewFromYAML(t, tt.configYaml)
			testConfig, err := newConfig(mockConfig)
			assert.Nil(t, err)
			assert.Equal(t, tt.expectedConfig.Namespace, testConfig.Namespace)
			assert.Equal(t, tt.expectedConfig.Devices, testConfig.Devices)
		})
	}
}

func TestConfig_Errors(t *testing.T) {
	var tests = []struct {
		name        string
		configYaml  string
		expectedErr string
	}{
		{
			name: "NCM malformed config, wrong type for devices (string instead of map)",
			configYaml: `
network_device_config_management:
  namespace: test
  devices: blah`,
			expectedErr: "'devices[0]' expected a map, got 'string'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := mock.NewFromYAML(t, tt.configYaml)
			_, err := newConfig(mockConfig)
			assert.NotNil(t, err)
			assert.ErrorContains(t, err, tt.expectedErr)
		})
	}
}
