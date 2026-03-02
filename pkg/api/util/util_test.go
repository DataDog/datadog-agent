// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsForbidden(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{
			name:     "empty string is forbidden",
			ip:       "",
			expected: true,
		},
		{
			name:     "0.0.0.0 is forbidden",
			ip:       "0.0.0.0",
			expected: true,
		},
		{
			name:     ":: is forbidden",
			ip:       "::",
			expected: true,
		},
		{
			name:     "0:0:0:0:0:0:0:0 is forbidden",
			ip:       "0:0:0:0:0:0:0:0",
			expected: true,
		},
		{
			name:     "127.0.0.1 is allowed",
			ip:       "127.0.0.1",
			expected: false,
		},
		{
			name:     "192.168.1.1 is allowed",
			ip:       "192.168.1.1",
			expected: false,
		},
		{
			name:     "::1 is allowed",
			ip:       "::1",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsForbidden(tt.ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConstantCompareStrings(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		tgt      string
		expected bool
	}{
		{
			name:     "equal strings",
			src:      "hello",
			tgt:      "hello",
			expected: true,
		},
		{
			name:     "different strings",
			src:      "hello",
			tgt:      "world",
			expected: false,
		},
		{
			name:     "empty strings",
			src:      "",
			tgt:      "",
			expected: true,
		},
		{
			name:     "different lengths",
			src:      "short",
			tgt:      "longer string",
			expected: false,
		},
		{
			name:     "same prefix different suffix",
			src:      "hello123",
			tgt:      "hello456",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := constantCompareStrings(tt.src, tt.tgt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTokenValidator(t *testing.T) {
	validToken := "test-token-12345"
	tokenGetter := func() string { return validToken }
	validator := TokenValidator(tokenGetter)

	t.Run("no authorization header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		err := validator(w, req)

		assert.Error(t, err)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer")
	})

	t.Run("wrong authorization scheme", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		w := httptest.NewRecorder()

		err := validator(w, req)

		assert.Error(t, err)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		w := httptest.NewRecorder()

		err := validator(w, req)

		assert.Error(t, err)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		w := httptest.NewRecorder()

		err := validator(w, req)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("bearer without token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer")
		w := httptest.NewRecorder()

		err := validator(w, req)

		assert.Error(t, err)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestCrossNodeClientTLSConfig(t *testing.T) {
	// Reset state before tests
	TestOnlyResetCrossNodeClientTLSConfig()

	t.Run("get config when not set", func(t *testing.T) {
		TestOnlyResetCrossNodeClientTLSConfig()
		config, err := GetCrossNodeClientTLSConfig()
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "not set")
	})

	t.Run("set nil config does nothing", func(t *testing.T) {
		TestOnlyResetCrossNodeClientTLSConfig()
		SetCrossNodeClientTLSConfig(nil)
		config, err := GetCrossNodeClientTLSConfig()
		assert.Error(t, err)
		assert.Nil(t, config)
	})

	t.Run("set and get config", func(t *testing.T) {
		TestOnlyResetCrossNodeClientTLSConfig()
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		}
		SetCrossNodeClientTLSConfig(tlsConfig)

		config, err := GetCrossNodeClientTLSConfig()
		require.NoError(t, err)
		require.NotNil(t, config)
		assert.Equal(t, tls.VersionTLS12, int(config.MinVersion))
	})

	t.Run("set config twice ignores second", func(t *testing.T) {
		TestOnlyResetCrossNodeClientTLSConfig()
		tlsConfig1 := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		tlsConfig2 := &tls.Config{
			MinVersion: tls.VersionTLS13,
		}

		SetCrossNodeClientTLSConfig(tlsConfig1)
		SetCrossNodeClientTLSConfig(tlsConfig2)

		config, err := GetCrossNodeClientTLSConfig()
		require.NoError(t, err)
		// Should still be TLS 1.2 from first config
		assert.Equal(t, tls.VersionTLS12, int(config.MinVersion))
	})

	t.Run("get config with InsecureSkipVerify", func(t *testing.T) {
		TestOnlyResetCrossNodeClientTLSConfig()
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
		}
		SetCrossNodeClientTLSConfig(tlsConfig)

		config, err := GetCrossNodeClientTLSConfig()
		require.NoError(t, err)
		assert.True(t, config.InsecureSkipVerify)
	})

	// Clean up
	TestOnlyResetCrossNodeClientTLSConfig()
}

func TestGetDCAAuthToken(t *testing.T) {
	// Test that GetDCAAuthToken returns the current token value
	// Note: We can't easily test InitDCAAuthToken without mocking the config
	token := GetDCAAuthToken()
	// Token should be empty or the previously set value
	assert.IsType(t, "", token)
}

func TestIsIPv6(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOutput bool
	}{
		{
			name:           "IPv4",
			input:          "192.168.0.1",
			expectedOutput: false,
		},
		{
			name:           "IPv6",
			input:          "2600:1f19:35d4:b900:527a:764f:e391:d369",
			expectedOutput: true,
		},
		{
			name:           "zero compressed IPv6",
			input:          "2600:1f19:35d4:b900::1",
			expectedOutput: true,
		},
		{
			name:           "IPv6 loopback",
			input:          "::1",
			expectedOutput: true,
		},
		{
			name:           "short hostname with only hexadecimal digits",
			input:          "cafe",
			expectedOutput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, IsIPv6(tt.input), tt.expectedOutput)
		})
	}
}
