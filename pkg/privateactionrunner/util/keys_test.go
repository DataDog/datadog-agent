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
			name:   "valid 40-char lowercase hex key",
			key:    "1234567890abcdef1234567890abcdef12345678",
			wantOK: true,
		},
		{
			name:   "valid ddapp_ key (ddapp_ + 34 alphanum)",
			key:    "ddapp_1234567890ABCDEFabcdef1234abcdef12",
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
			name:   "uppercase hex rejected (must be lowercase)",
			key:    "1234567890ABCDEF1234567890ABCDEF12345678",
			wantOK: false,
		},
		{
			name:   "pub prefix rejected",
			key:    "pub1234567890abcdef1234567890abcdef12def789",
			wantOK: false,
		},
		{
			name:   "invalid characters in hex section",
			key:    "1234567890abcdef1234567890abcdefXXYYZZWW",
			wantOK: false,
		},
		{
			name:   "ddapp_ too short (33 alphanum instead of 34)",
			key:    "ddapp_1234567890ABCDEFabcdef1234abc",
			wantOK: false,
		},
		{
			name:   "ddapp_ too long (35 alphanum instead of 34)",
			key:    "ddapp_1234567890ABCDEFabcdef1234abcde",
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
