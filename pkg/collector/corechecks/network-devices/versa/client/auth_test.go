// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthMethodParse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    authMethod
		expectError bool
	}{
		{
			name:        "Valid basic auth method",
			input:       "basic",
			expected:    authMethodBasic,
			expectError: false,
		},
		{
			name:        "Valid OAuth auth method",
			input:       "oauth",
			expected:    authMethodOAuth,
			expectError: false,
		},
		{
			name:        "Valid basic auth with capitalization",
			input:       "BaSiC",
			expected:    authMethodBasic,
			expectError: false,
		},
		{
			name:        "Valid oauth with capitalization",
			input:       "OAuth",
			expected:    authMethodOAuth,
			expectError: false,
		},
		{
			name:        "Invalid auth method - random string",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "Invalid auth method - empty string",
			input:       "",
			expectError: true,
		},
		{
			name:        "Invalid auth method - typo",
			input:       "basci",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var method authMethod
			err := method.Parse(tt.input)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid auth method")
				assert.Contains(t, err.Error(), tt.input)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, method)
			}
		})
	}
}

func TestProcessAuthConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      AuthConfig
		expected    authMethod
		expectError bool
	}{
		{
			name: "Valid basic auth config",
			config: AuthConfig{
				Method:   "basic",
				Username: "testuser",
				Password: "testpass",
			},
			expected:    authMethodBasic,
			expectError: false,
		},
		{
			name: "Valid OAuth config with credentials",
			config: AuthConfig{
				Method:       "oauth",
				Username:     "testuser",
				Password:     "testpass",
				ClientID:     "test-client-id",
				ClientSecret: "test-client-secret",
			},
			expected:    authMethodOAuth,
			expectError: false,
		},
		{
			name: "Empty method defaults to basic",
			config: AuthConfig{
				Method:   "", // Empty should default to basic
				Username: "testuser",
				Password: "testpass",
			},
			expected:    authMethodBasic,
			expectError: false,
		},
		{
			name: "Invalid OAuth config - missing client ID",
			config: AuthConfig{
				Method:       "oauth",
				Username:     "testuser",
				Password:     "testpass",
				ClientSecret: "test-client-secret",
			},
			expectError: true,
		},
		{
			name: "Invalid OAuth config - missing client secret",
			config: AuthConfig{
				Method:   "oauth",
				Username: "testuser",
				Password: "testpass",
				ClientID: "test-client-id",
			},
			expectError: true,
		},
		{
			name: "Invalid config - missing username",
			config: AuthConfig{
				Method:   "basic",
				Password: "testpass",
			},
			expectError: true,
		},
		{
			name: "Invalid config - missing password",
			config: AuthConfig{
				Method:   "basic",
				Username: "testuser",
			},
			expectError: true,
		},
		{
			name: "Invalid auth method",
			config: AuthConfig{
				Method:   "invalid",
				Username: "testuser",
				Password: "testpass",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processAuthConfig(tt.config)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
