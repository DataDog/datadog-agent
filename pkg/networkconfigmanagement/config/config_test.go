// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestDeviceConfig_Validation(t *testing.T) {
	tests := []struct {
		name        string
		config      DeviceConfig
		expectValid bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: DeviceConfig{
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
			config: DeviceConfig{
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
			config: DeviceConfig{
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
			config: DeviceConfig{
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
			name: "missing password",
			config: DeviceConfig{
				IPAddress: "100.1.1.1",
				Auth: AuthCredentials{
					Username: "admin",
					Port:     "22",
					Protocol: "tcp",
				},
			},
			expectValid: false,
			errorMsg:    "auth is required: missing password",
		},
		{
			name: "invalid port",
			config: DeviceConfig{
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
			config: DeviceConfig{
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
			err := tt.config.ValidateConfig()
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

func TestDeviceConfig_YAML_Marshaling(t *testing.T) {
	config := DeviceConfig{
		Namespace: "test",
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
	var parsed DeviceConfig
	err = yaml.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, config.IPAddress, parsed.IPAddress)
	assert.Equal(t, config.Auth.Username, parsed.Auth.Username)
	assert.Equal(t, config.Auth.Password, parsed.Auth.Password)
}

func TestAuthCredentials_DefaultValues(t *testing.T) {
	config := DeviceConfig{
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
