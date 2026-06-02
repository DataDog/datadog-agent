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

// unsetEnv unsets an environment variable for the duration of the test,
// restoring the original value (or absence) on cleanup.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	orig, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("failed to unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			os.Setenv(key, orig)
		} else {
			os.Unsetenv(key)
		}
	})
}

func TestValidateDurationEnvVars(t *testing.T) {
	tests := []struct {
		name               string
		envVars            map[string]string
		wantErr            bool
		wantErrContains    []string
		wantErrNotContains []string
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
			name:    "valid zero duration with suffix",
			envVars: map[string]string{"DD_SYNC_DELAY": "0s"},
			wantErr: false,
		},
		{
			name:    "bare zero is valid without suffix",
			envVars: map[string]string{"DD_SYNC_DELAY": "0"},
			wantErr: false,
		},
		{
			name:    "bare number without unit suffix",
			envVars: map[string]string{"DD_SYNC_DELAY": "30"},
			wantErr: true,
			wantErrContains: []string{
				`invalid value "30"`,
				"DD_SYNC_DELAY",
				"--sync-delay",
				`did you mean "30s"`,
			},
		},
		{
			name:    "bare floating point without unit suffix",
			envVars: map[string]string{"DD_SYNC_TO": "2.5"},
			wantErr: true,
			wantErrContains: []string{
				`invalid value "2.5"`,
				"DD_SYNC_TO",
				"--sync-to",
				`did you mean "2.5s"`,
			},
		},
		{
			name:    "non-numeric invalid string",
			envVars: map[string]string{"DD_SYNC_DELAY": "abc"},
			wantErr: true,
			wantErrContains: []string{
				`invalid value "abc"`,
				"DD_SYNC_DELAY",
			},
			wantErrNotContains: []string{
				"did you mean",
			},
		},
		{
			name:    "empty value is invalid",
			envVars: map[string]string{"DD_SYNC_DELAY": ""},
			wantErr: true,
			wantErrContains: []string{
				`invalid value ""`,
				"DD_SYNC_DELAY",
			},
			wantErrNotContains: []string{
				"did you mean",
			},
		},
		{
			name:    "both valid durations",
			envVars: map[string]string{"DD_SYNC_DELAY": "30s", "DD_SYNC_TO": "5s"},
			wantErr: false,
		},
		{
			name:    "both invalid durations reports all errors",
			envVars: map[string]string{"DD_SYNC_DELAY": "30", "DD_SYNC_TO": "5"},
			wantErr: true,
			wantErrContains: []string{
				"DD_SYNC_DELAY",
				"DD_SYNC_TO",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, d := range durationEnvVars {
				unsetEnv(t, d.envKey)
			}
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			err := validateDurationEnvVars()
			if tt.wantErr {
				require.Error(t, err)
				for i, s := range tt.wantErrContains {
					assert.Contains(t, err.Error(), s,
						"wantErrContains[%d]: expected error to contain %q", i, s)
				}
				for i, s := range tt.wantErrNotContains {
					assert.NotContains(t, err.Error(), s,
						"wantErrNotContains[%d]: expected error NOT to contain %q", i, s)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
