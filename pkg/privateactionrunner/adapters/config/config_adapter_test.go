// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestIsActionAllowed(t *testing.T) {
	tests := []struct {
		name             string
		actionsAllowlist map[string]sets.Set[string]
		bundleId         string
		actionName       string
		expected         bool
	}{
		{
			name: "action allowed - exact match",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("runScript", "testConnection"),
			},
			bundleId:   "com.datadoghq.script",
			actionName: "runScript",
			expected:   true,
		},
		{
			name: "action allowed - wildcard",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("*"),
			},
			bundleId:   "com.datadoghq.script",
			actionName: "anyAction",
			expected:   true,
		},
		{
			name: "action not allowed - not in set",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("runScript"),
			},
			bundleId:   "com.datadoghq.script",
			actionName: "deleteScript",
			expected:   false,
		},
		{
			name: "bundle not in allowlist",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.other": sets.New[string]("action1"),
			},
			bundleId:   "com.datadoghq.script",
			actionName: "runScript",
			expected:   false,
		},
		{
			name:             "empty allowlist",
			actionsAllowlist: map[string]sets.Set[string]{},
			bundleId:         "com.datadoghq.script",
			actionName:       "runScript",
			expected:         false,
		},
		{
			name:             "nil allowlist",
			actionsAllowlist: nil,
			bundleId:         "com.datadoghq.script",
			actionName:       "runScript",
			expected:         false,
		},
		{
			name: "empty action set for bundle",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string](),
			},
			bundleId:   "com.datadoghq.script",
			actionName: "runScript",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				ActionsAllowlist: tt.actionsAllowlist,
			}
			result := cfg.IsActionAllowed(tt.bundleId, tt.actionName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsURLInAllowlist(t *testing.T) {
	tests := []struct {
		name      string
		allowlist []string
		url       string
		expected  bool
	}{
		{
			name:      "nil allowlist allows all",
			allowlist: nil,
			url:       "https://example.com/path",
			expected:  true,
		},
		{
			name:      "exact match",
			allowlist: []string{"example.com"},
			url:       "https://example.com/path",
			expected:  true,
		},
		{
			name:      "case insensitive match",
			allowlist: []string{"EXAMPLE.COM"},
			url:       "https://example.com/path",
			expected:  true,
		},
		{
			name:      "glob pattern match - wildcard subdomain",
			allowlist: []string{"*.example.com"},
			url:       "https://api.example.com/path",
			expected:  true,
		},
		{
			name:      "glob pattern match - double wildcard",
			allowlist: []string{"**.datadoghq.com"},
			url:       "https://app.us1.datadoghq.com/path",
			expected:  true,
		},
		{
			name:      "no match",
			allowlist: []string{"example.com"},
			url:       "https://other.com/path",
			expected:  false,
		},
		{
			name:      "invalid URL",
			allowlist: []string{"example.com"},
			url:       "://invalid",
			expected:  false,
		},
		{
			name:      "multiple patterns - match second",
			allowlist: []string{"first.com", "second.com", "third.com"},
			url:       "https://second.com/path",
			expected:  true,
		},
		{
			name:      "empty allowlist",
			allowlist: []string{},
			url:       "https://example.com/path",
			expected:  false,
		},
		{
			name:      "URL with port",
			allowlist: []string{"example.com"},
			url:       "https://example.com:8080/path",
			expected:  true,
		},
		{
			name:      "URL with query params",
			allowlist: []string{"example.com"},
			url:       "https://example.com/path?query=value",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Allowlist: tt.allowlist,
			}
			result := cfg.IsURLInAllowlist(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIdentityIsIncomplete(t *testing.T) {
	// Generate a test key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tests := []struct {
		name       string
		urn        string
		privateKey *ecdsa.PrivateKey
		expected   bool
	}{
		{
			name:       "complete identity",
			urn:        "urn:dd:apps:on-prem-runner:us1:12345:runner-id",
			privateKey: privateKey,
			expected:   false,
		},
		{
			name:       "missing URN",
			urn:        "",
			privateKey: privateKey,
			expected:   true,
		},
		{
			name:       "missing private key",
			urn:        "urn:dd:apps:on-prem-runner:us1:12345:runner-id",
			privateKey: nil,
			expected:   true,
		},
		{
			name:       "both missing",
			urn:        "",
			privateKey: nil,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Urn:        tt.urn,
				PrivateKey: tt.privateKey,
			}
			result := cfg.IdentityIsIncomplete()
			assert.Equal(t, tt.expected, result)
		})
	}
}
