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
	"gopkg.in/yaml.v2"
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
