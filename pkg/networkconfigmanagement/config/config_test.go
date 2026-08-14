// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

func TestDeviceInstance_Validation(t *testing.T) {
	tests := []struct {
		name        string
		config      DeviceInstance
		expectValid bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: DeviceInstance{
				IPAddress: "100.1.1.1",
				Auth: AuthCredentials{
					Username: "admin",
					Password: "password",
					Port:     "22",
					Protocol: "tcp",
					SSH: &SSHConfig{
						InsecureSkipVerify: true,
					},
				},
			},
			expectValid: true,
		},
		{
			name: "missing IP address",
			config: DeviceInstance{
				Auth: AuthCredentials{
					Username: "admin",
					Password: "password",
					Port:     "22",
					Protocol: "tcp",
					SSH: &SSHConfig{
						InsecureSkipVerify: true,
					},
				},
			},
			expectValid: false,
			errorMsg:    "ip_address is required",
		},
		{
			name: "invalid IP address",
			config: DeviceInstance{
				IPAddress: "not-an-ip",
				Auth: AuthCredentials{
					Username: "admin",
					Password: "password",
					Port:     "22",
					Protocol: "tcp",
					SSH: &SSHConfig{
						InsecureSkipVerify: true,
					},
				},
			},
			expectValid: false,
			errorMsg:    "invalid ip_address format",
		},
		{
			name: "missing username",
			config: DeviceInstance{
				IPAddress: "100.1.1.1",
				Auth: AuthCredentials{
					Password: "password",
					Port:     "22",
					Protocol: "tcp",
					SSH: &SSHConfig{
						InsecureSkipVerify: true,
					},
				},
			},
			expectValid: false,
			errorMsg:    "auth is required: missing username",
		},
		{
			name: "missing auth method (no password/private key)",
			config: DeviceInstance{
				IPAddress: "100.1.1.1",
				Auth: AuthCredentials{
					Username: "admin",
					Port:     "22",
					Protocol: "tcp",
					SSH: &SSHConfig{
						InsecureSkipVerify: true,
					},
				},
			},
			expectValid: false,
			errorMsg:    "auth is required: missing auth method (either password or private key) for device 100.1.1.1",
		},
		{
			name: "invalid port",
			config: DeviceInstance{
				IPAddress: "100.1.1.1",
				Auth: AuthCredentials{
					Username: "admin",
					Password: "password",
					Port:     "not-a-port",
					Protocol: "tcp",
					SSH: &SSHConfig{
						InsecureSkipVerify: true,
					},
				},
			},
			expectValid: false,
			errorMsg:    "invalid port, not valid integer",
		},
		{
			name: "port out of range",
			config: DeviceInstance{
				IPAddress: "100.1.1.1",
				Auth: AuthCredentials{
					Username: "admin",
					Password: "password",
					Port:     "99999",
					Protocol: "tcp",
					SSH: &SSHConfig{
						InsecureSkipVerify: true,
					},
				},
			},
			expectValid: false,
			errorMsg:    "invalid port, out of range",
		},
		{
			name: "missing SSH config",
			config: DeviceInstance{
				IPAddress: "100.1.1.1",
				Auth: AuthCredentials{
					Username: "admin",
					Password: "password",
					Port:     "22",
					Protocol: "tcp",
				},
			},
			expectValid: false,
			errorMsg:    "auth is required: missing SSH configuration for device 100.1.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

func TestSSHConfig_Validation(t *testing.T) {
	tests := []struct {
		name           string
		config         *SSHConfig
		expectedConfig *SSHConfig
		errMsg         string
	}{
		{
			name: "valid config: known_path is set",
			config: &SSHConfig{
				KnownHostsPath: "/test/directory",
				Timeout:        60 * time.Second,
			},
			expectedConfig: &SSHConfig{
				KnownHostsPath: "/test/directory",
				Timeout:        60 * time.Second,
			},
		},
		{
			name: "valid config, missing timeout uses default",
			config: &SSHConfig{
				InsecureSkipVerify: true,
			},
			expectedConfig: &SSHConfig{
				InsecureSkipVerify: true,
				Timeout:            defaultSSHTimeout,
			},
		},
		{
			name:           "missing both known_hosts path and insecure_skip_verify",
			config:         &SSHConfig{},
			expectedConfig: &SSHConfig{},
			errMsg:         "no SSH host key verification configured: set known_hosts_path or enable insecure_skip_verify",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if tt.errMsg != "" {
				assert.EqualError(t, err, tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeviceInstance_YAML_Marshaling(t *testing.T) {
	config := DeviceInstance{
		IPAddress: "10.100.1.1",
		Auth: AuthCredentials{
			Username: "admin",
			Password: "password",
			Port:     "22",
			Protocol: "tcp",
		},
	}

	// Test marshaling
	data, err := yaml.Marshal(config)
	require.NoError(t, err)
	assert.Contains(t, string(data), "ip_address: 10.100.1.1")
	assert.Contains(t, string(data), "username: admin")

	// Test unmarshaling
	var parsed DeviceInstance
	err = yaml.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, config.IPAddress, parsed.IPAddress)
	assert.Equal(t, config.Auth.Username, parsed.Auth.Username)
	assert.Equal(t, config.Auth.Password, parsed.Auth.Password)
}

func TestAuthCredentials_DefaultValues(t *testing.T) {
	config := DeviceInstance{
		IPAddress: "10.100.1.1",
		Auth: AuthCredentials{
			Username: "admin",
			Password: "password",
		},
	}
	ic := &InitConfig{
		Namespace: "thenamespace",
		SSH: &SSHConfig{
			InsecureSkipVerify: true,
		},
	}
	config.applyDefaults(ic)
	assert.Equal(t, "22", config.Auth.Port)
	assert.Equal(t, "tcp", config.Auth.Protocol)
	assert.Equal(t, "thenamespace", config.Namespace)
	assert.True(t, config.Auth.SSH.InsecureSkipVerify)
}

func TestInitConfig_InventoryReportMaxInterval_ApplyDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		{name: "unset (zero) defaults to 1h", input: 0, expected: defaultInventoryReportMaxInterval},
		{name: "negative defaults to 1h", input: -5 * time.Minute, expected: defaultInventoryReportMaxInterval},
		{name: "user-set value preserved", input: 7 * time.Hour, expected: 7 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ic := InitConfig{
				MinCollectionInterval:      60,
				InventoryReportMaxInterval: tt.input,
			}
			ic.applyDefaults()
			assert.Equal(t, tt.expected, ic.InventoryReportMaxInterval)
		})
	}
}

func TestInitConfig_InventoryReportMaxInterval_Validate(t *testing.T) {
	tests := []struct {
		name    string
		value   time.Duration
		wantErr string
	}{
		{name: "zero rejected", value: 0, wantErr: "inventory_report_max_interval must be greater than 0"},
		{name: "negative rejected", value: -1 * time.Second, wantErr: "inventory_report_max_interval must be greater than 0"},
		{name: "positive accepted", value: 1 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ic := InitConfig{
				Namespace:                  "default",
				MinCollectionInterval:      60,
				InventoryReportMaxInterval: tt.value,
			}
			err := ic.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParsingSSHTimeoutFromYAML(t *testing.T) {
	var tests = []struct {
		name     string
		timeout  string
		expected time.Duration
	}{
		{
			// we directed customers to specify this option as a timeout string, and want to maintain backwards compatibility for now.
			name:     "duration string with units",
			timeout:  "1m30s",
			expected: 90 * time.Second,
		},
		{
			name:     "duration string without units",
			timeout:  "60",
			expected: 60 * time.Second,
		},
		{
			name:     "duration string with only seconds unit",
			timeout:  "45s",
			expected: 45 * time.Second,
		},
		{
			name:     "upper boundary for conversion",
			timeout:  "999999",
			expected: 999999 * time.Second,
		},
		{
			name:     "upper boundary for conversion",
			timeout:  "1000000",
			expected: time.Millisecond,
		},
		{
			name:     "negative duration string",
			timeout:  "-30s",
			expected: defaultSSHTimeout, // should fall back to default on error
		},
	}

	var newConfigs = func(timeout string) ([]byte, []byte) {
		initConfig := `
namespace: default
ssh:
  insecure_skip_verify: true
  timeout: ` + timeout + `
`
		instanceConfig := `
ip_address: 192.168.0.1
auth:
  password: 'password'
  username: 'admin'
`
		return []byte(initConfig), []byte(instanceConfig)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initConfig, instanceConfig := newConfigs(tt.timeout)
			cfg, err := NewNcmCheckContext(instanceConfig, initConfig)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.Device.Auth.SSH.Timeout)
		})
	}
}
