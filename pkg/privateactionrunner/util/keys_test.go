// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAppKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantOK  bool
		wantErr bool
	}{
		// --- valid keys ---
		{
			name:   "valid hex key (34 hex + def789)",
			key:    "1234567890abcdef1234567890abcdef12def789",
			wantOK: true,
		},
		{
			name:   "valid hex key with pub prefix",
			key:    "pub1234567890abcdef1234567890abcdef12def789",
			wantOK: true,
		},
		{
			name:   "valid hex key with PUB prefix (case-insensitive)",
			key:    "PUB1234567890abcdef1234567890abcdef12def789",
			wantOK: true,
		},
		{
			name:   "valid hex key ending DEF789 (case-insensitive suffix)",
			key:    "1234567890abcdef1234567890abcdef12DEF789",
			wantOK: true,
		},
		{
			name:   "valid ddapp_ key",
			key:    "ddapp_1234567890ABCDEFabcdef1234abdef789",
			wantOK: true,
		},
		// --- soft failures (false, nil) ---
		{
			name:   "empty key",
			wantOK: false,
		},
		{
			name:   "too short",
			key:    "abc123",
			wantOK: false,
		},
		{
			name:   "wrong length hex (40 chars but no def789 suffix)",
			key:    "1234567890abcdef1234567890abcdef12345678",
			wantOK: false,
		},
		{
			name:   "invalid characters in hex section",
			key:    "1234567890abcdef1234567890abcdefXXdef789",
			wantOK: false,
		},
		{
			name:   "ddapp_ wrong suffix",
			key:    "ddapp_1234567890ABCDEFabcdef1234xxxxxx",
			wantOK: false,
		},
		{
			name:   "ddapp_ too short",
			key:    "ddapp_short",
			wantOK: false,
		},
		// --- hard failures (false, error) ---
		{
			name:    "unresolved ENC secret",
			key:     "ENC[my_secret_name]",
			wantOK:  false,
			wantErr: true,
		},
		{
			name:    "ENC secret with surrounding whitespace",
			key:     "  ENC[app_key_secret]  ",
			wantOK:  false,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := ValidateAppKey(tc.key)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantOK  bool
		wantErr bool
	}{
		// --- valid ---
		{
			name:   "valid 32 hex chars",
			key:    "1234567890abcdef1234567890abcdef",
			wantOK: true,
		},
		{
			name:   "valid uppercase hex",
			key:    "1234567890ABCDEF1234567890ABCDEF",
			wantOK: true,
		},
		// --- soft failures ---
		{
			name:   "empty key",
			wantOK: false,
		},
		{
			name:   "too short (31 chars)",
			key:    "1234567890abcdef1234567890abcde",
			wantOK: false,
		},
		{
			name:   "too long (33 chars)",
			key:    "1234567890abcdef1234567890abcdef0",
			wantOK: false,
		},
		{
			name:   "invalid characters",
			key:    "1234567890abcdef1234567890abcdXX",
			wantOK: false,
		},
		// --- hard failures ---
		{
			name:    "unresolved ENC secret",
			key:     "ENC[api_key_secret]",
			wantOK:  false,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := ValidateAPIKey(tc.key)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAppKeyURLRegex(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantMatch bool
	}{
		{
			name:      "hex key in URL",
			url:       "https://example.com/api?application_key=1234567890abcdef1234567890abcdef12def789",
			wantMatch: true,
		},
		{
			name:      "case-insensitive param name",
			url:       "https://example.com/api?APPLICATION_KEY=1234567890abcdef1234567890abcdef12def789",
			wantMatch: true,
		},
		{
			name:      "key with pub prefix in URL",
			url:       "https://example.com/api?application_key=pub1234567890abcdef1234567890abcdef12def789",
			wantMatch: true,
		},
		{
			name:      "ddapp_ key in URL",
			url:       "https://example.com/api?application_key=ddapp_1234567890ABCDEFabcdef1234abdef789",
			wantMatch: true,
		},
		{
			name:      "key among multiple params",
			url:       "https://example.com/api?foo=bar&application_key=1234567890abcdef1234567890abcdef12def789&baz=qux",
			wantMatch: true,
		},
		{
			name:      "invalid key value",
			url:       "https://example.com/api?application_key=tooshort",
			wantMatch: false,
		},
		{
			name:      "no query string",
			url:       "https://example.com/api",
			wantMatch: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantMatch, AppKeyURLRegex.MatchString(tc.url))
		})
	}
}
