// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package azure

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenExpiry(t *testing.T) {
	eastern := time.FixedZone("UTC-4", -4*60*60)
	expiry := time.Date(2026, 10, 7, 16, 17, 49, 0, time.UTC)

	tests := []struct {
		name string
		out  string
	}{
		{
			name: "epoch and local time",
			out:  `{"epoch": 1791389869, "local": "2026-10-07 12:17:49.000000"}`,
		},
		{
			// Azure CLI older than 2.54.0 has no expires_on field.
			name: "local time only",
			out:  `{"epoch": null, "local": "2026-10-07 12:17:49.000000"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tokenExpiry([]byte(tt.out), eastern)
			require.NoError(t, err)
			assert.True(t, got.Equal(expiry), "got %v, want %v", got, expiry)
			// Parsing expiresOn as UTC put the expiry four hours early.
			assert.False(t, got.Before(expiry.Add(-time.Hour)), "expiry %v is hours early", got)
		})
	}
}

func TestTokenExpiryRejectsUnexpectedOutput(t *testing.T) {
	for _, out := range []string{
		"",
		"2026-10-07 12:17:49.000000",
		`{"local": "tomorrow"}`,
	} {
		_, err := tokenExpiry([]byte(out), time.UTC)
		assert.Error(t, err, "output %q", out)
	}
}
