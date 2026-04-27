// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

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
				},
			},
			expectValid: false,
			errorMsg:    "invalid port, out of range",
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
	config.applyDefaults()
	assert.Equal(t, "22", config.Auth.Port)
	assert.Equal(t, "tcp", config.Auth.Protocol)
}

func TestStoreConfig_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config *StoreConfig
		errMsg string
	}{
		{
			name: "valid config",
			config: &StoreConfig{
				MinConfigsPerDevice:    2,
				MaxConfigsPerDevice:    50,
				MaxRawConfigStoreBytes: 512 * 1024 * 1024,
			},
		},
		{
			name: "min equals max is valid",
			config: &StoreConfig{
				MinConfigsPerDevice:    10,
				MaxConfigsPerDevice:    10,
				MaxRawConfigStoreBytes: 1024,
			},
		},
		{
			name: "min_configs_per_device zero",
			config: &StoreConfig{
				MinConfigsPerDevice:    0,
				MaxConfigsPerDevice:    50,
				MaxRawConfigStoreBytes: 512 * 1024 * 1024,
			},
			errMsg: "store.min_configs_per_device must be greater than zero",
		},
		{
			name: "max_configs_per_device zero",
			config: &StoreConfig{
				MinConfigsPerDevice:    2,
				MaxConfigsPerDevice:    0,
				MaxRawConfigStoreBytes: 512 * 1024 * 1024,
			},
			errMsg: "store.max_configs_per_device must be greater than zero",
		},
		{
			name: "min exceeds max",
			config: &StoreConfig{
				MinConfigsPerDevice:    100,
				MaxConfigsPerDevice:    10,
				MaxRawConfigStoreBytes: 512 * 1024 * 1024,
			},
			errMsg: "store.min_configs_per_device (100) must not exceed store.max_configs_per_device (10)",
		},
		{
			name: "max_raw_config_store_bytes zero",
			config: &StoreConfig{
				MinConfigsPerDevice:    2,
				MaxConfigsPerDevice:    50,
				MaxRawConfigStoreBytes: 0,
			},
			errMsg: "store.max_raw_config_store_bytes must be greater than zero",
		},
		{
			name: "max_raw_config_store_bytes negative",
			config: &StoreConfig{
				MinConfigsPerDevice:    2,
				MaxConfigsPerDevice:    50,
				MaxRawConfigStoreBytes: -1,
			},
			errMsg: "store.max_raw_config_store_bytes must be greater than zero",
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

func TestStoreConfig_Defaults(t *testing.T) {
	ic := InitConfig{
		Namespace: "default",
	}
	ic.applyDefaults()

	require.NotNil(t, ic.Store)
	assert.Equal(t, defaultMinConfigsPerDevice, ic.Store.MinConfigsPerDevice)
	assert.Equal(t, defaultMaxConfigsPerDevice, ic.Store.MaxConfigsPerDevice)
	assert.Equal(t, defaultMaxRawConfigStoreBytes, ic.Store.MaxRawConfigStoreBytes)
}

func TestStoreConfig_PartialDefaults(t *testing.T) {
	ic := InitConfig{
		Namespace: "default",
		Store: &StoreConfig{
			MinConfigsPerDevice: 5,
		},
	}
	ic.applyDefaults()

	assert.Equal(t, 5, ic.Store.MinConfigsPerDevice)
	assert.Equal(t, defaultMaxConfigsPerDevice, ic.Store.MaxConfigsPerDevice)
	assert.Equal(t, defaultMaxRawConfigStoreBytes, ic.Store.MaxRawConfigStoreBytes)
}

func TestStoreConfig_YAMLUnmarshal(t *testing.T) {
	yamlData := `
namespace: production
ssh:
  insecure_skip_verify: true
store:
  min_configs_per_device: 3
  max_configs_per_device: 100
  max_raw_config_store_bytes: 1073741824
`
	var ic InitConfig
	err := yaml.Unmarshal([]byte(yamlData), &ic)
	require.NoError(t, err)
	require.NotNil(t, ic.Store)
	assert.Equal(t, 3, ic.Store.MinConfigsPerDevice)
	assert.Equal(t, 100, ic.Store.MaxConfigsPerDevice)
	assert.Equal(t, int64(1073741824), ic.Store.MaxRawConfigStoreBytes)
}

func TestStoreConfig_YAMLUnmarshalOmitted(t *testing.T) {
	yamlData := `
namespace: production
ssh:
  insecure_skip_verify: true
`
	var ic InitConfig
	err := yaml.Unmarshal([]byte(yamlData), &ic)
	require.NoError(t, err)
	assert.Nil(t, ic.Store)

	ic.applyDefaults()
	require.NotNil(t, ic.Store)
	assert.Equal(t, defaultMinConfigsPerDevice, ic.Store.MinConfigsPerDevice)
}

func TestNewNcmCheckContext_WithStoreConfig(t *testing.T) {
	initConfig := `
namespace: default
ssh:
  insecure_skip_verify: true
store:
  min_configs_per_device: 5
  max_configs_per_device: 200
  max_raw_config_store_bytes: 2147483648
`
	instanceConfig := `
ip_address: 192.168.0.1
auth:
  password: 'password'
  username: 'admin'
`
	cfg, err := NewNcmCheckContext([]byte(instanceConfig), []byte(initConfig))
	require.NoError(t, err)
	require.NotNil(t, cfg.Store)
	assert.Equal(t, 5, cfg.Store.MinConfigsPerDevice)
	assert.Equal(t, 200, cfg.Store.MaxConfigsPerDevice)
	assert.Equal(t, int64(2147483648), cfg.Store.MaxRawConfigStoreBytes)
}

func TestNewNcmCheckContext_StoreDefaultsApplied(t *testing.T) {
	initConfig := `
namespace: default
ssh:
  insecure_skip_verify: true
`
	instanceConfig := `
ip_address: 192.168.0.1
auth:
  password: 'password'
  username: 'admin'
`
	cfg, err := NewNcmCheckContext([]byte(instanceConfig), []byte(initConfig))
	require.NoError(t, err)
	require.NotNil(t, cfg.Store)
	assert.Equal(t, defaultMinConfigsPerDevice, cfg.Store.MinConfigsPerDevice)
	assert.Equal(t, defaultMaxConfigsPerDevice, cfg.Store.MaxConfigsPerDevice)
	assert.Equal(t, defaultMaxRawConfigStoreBytes, cfg.Store.MaxRawConfigStoreBytes)
}

func TestNewNcmCheckContext_InvalidStoreConfig(t *testing.T) {
	initConfig := `
namespace: default
ssh:
  insecure_skip_verify: true
store:
  min_configs_per_device: 100
  max_configs_per_device: 10
  max_raw_config_store_bytes: 1024
`
	instanceConfig := `
ip_address: 192.168.0.1
auth:
  password: 'password'
  username: 'admin'
`
	_, err := NewNcmCheckContext([]byte(instanceConfig), []byte(initConfig))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store.min_configs_per_device (100) must not exceed store.max_configs_per_device (10)")
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
