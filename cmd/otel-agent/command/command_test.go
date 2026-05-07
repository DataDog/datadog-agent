// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package command

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDurationEnvVars(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		wantErr     bool
		errContains []string
	}{
		{
			name:    "no env vars set",
			envVars: map[string]string{},
			wantErr: false,
		},
		{
			name:    "valid duration with seconds suffix",
			envVars: map[string]string{"DD_SYNC_DELAY": "30s"},
			wantErr: false,
		},
		{
			name:    "valid duration with minutes suffix",
			envVars: map[string]string{"DD_SYNC_DELAY": "1m"},
			wantErr: false,
		},
		{
			name:    "valid complex duration",
			envVars: map[string]string{"DD_SYNC_DELAY": "1m30s"},
			wantErr: false,
		},
		{
			name:    "valid zero duration",
			envVars: map[string]string{"DD_SYNC_DELAY": "0s"},
			wantErr: false,
		},
		{
			name:    "bare number without unit suffix",
			envVars: map[string]string{"DD_SYNC_DELAY": "30"},
			wantErr: true,
			errContains: []string{
				`invalid value "30"`,
				"DD_SYNC_DELAY",
				"missing unit in duration",
				`did you mean "30s"`,
			},
		},
		{
			name:    "bare floating point without unit suffix",
			envVars: map[string]string{"DD_SYNC_TO": "2.5"},
			wantErr: true,
			errContains: []string{
				`invalid value "2.5"`,
				"DD_SYNC_TO",
				`did you mean "2.5s"`,
			},
		},
		{
			name:    "non-numeric invalid string",
			envVars: map[string]string{"DD_SYNC_DELAY": "abc"},
			wantErr: true,
			errContains: []string{
				`invalid value "abc"`,
				"DD_SYNC_DELAY",
				"invalid duration",
			},
		},
		{
			name:    "empty value is ignored",
			envVars: map[string]string{"DD_SYNC_DELAY": ""},
			wantErr: false,
		},
		{
			name:    "both valid durations",
			envVars: map[string]string{"DD_SYNC_DELAY": "30s", "DD_SYNC_TO": "5s"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for envVar := range durationEnvVars {
				t.Setenv(envVar, "")
				os.Unsetenv(envVar)
			}
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			err := validateDurationEnvVars()
			if tt.wantErr {
				require.Error(t, err)
				for _, s := range tt.errContains {
					assert.Contains(t, err.Error(), s)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
