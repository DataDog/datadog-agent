// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procmgrdecision

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

// parseInputs are the values of the table in section 7 of the Phase 1 spec.
var parseInputs = []string{
	"true", "false", "TRUE", "False", " false ", "", "   ", "yes", "0", "1", "no", " true ", "FALSE", "garbage",
}

func TestParseStrict(t *testing.T) {
	tests := []struct {
		raw     string
		enabled bool
		set     bool
		wantErr bool
	}{
		{raw: "true", enabled: true, set: true},
		{raw: "TRUE", enabled: true, set: true},
		{raw: " true ", enabled: true, set: true},
		{raw: "false", enabled: false, set: true},
		{raw: "False", enabled: false, set: true},
		{raw: " false ", enabled: false, set: true},
		{raw: ""},
		{raw: "   "},
		{raw: "yes", wantErr: true},
		{raw: "0", wantErr: true},
		{raw: "1", wantErr: true},
		{raw: "no", wantErr: true},
	}
	for _, tt := range tests {
		t.Run("'"+tt.raw+"'", func(t *testing.T) {
			enabled, set, err := ParseStrict(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "DD_PROCESS_MANAGER_ENABLED")
				assert.Contains(t, err.Error(), "true and false")
				assert.False(t, set)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.enabled, enabled)
			assert.Equal(t, tt.set, set)
		})
	}
}

func TestParseStrictErrorNamesValue(t *testing.T) {
	_, _, err := ParseStrict("maybe")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"maybe"`)
}

func TestParseLenient(t *testing.T) {
	tests := []struct {
		raw     string
		enabled bool
		set     bool
		warning bool
	}{
		{raw: "true", enabled: true, set: true},
		{raw: "TRUE", enabled: true, set: true},
		{raw: "false", enabled: false, set: true},
		{raw: "False", enabled: false, set: true},
		{raw: " false ", enabled: false, set: true},
		{raw: ""},
		{raw: "   "},
		{raw: "yes", enabled: true, set: true, warning: true},
		{raw: "0", enabled: true, set: true, warning: true},
		{raw: "1", enabled: true, set: true, warning: true},
		{raw: "no", enabled: true, set: true, warning: true},
	}
	for _, tt := range tests {
		t.Run("'"+tt.raw+"'", func(t *testing.T) {
			enabled, set, warning := ParseLenient(tt.raw)
			assert.Equal(t, tt.set, set)
			if tt.set {
				assert.Equal(t, tt.enabled, enabled)
			}
			if tt.warning {
				assert.NotEmpty(t, warning)
			} else {
				assert.Empty(t, warning)
			}
		})
	}
}

func TestParseLenientWarningNamesValue(t *testing.T) {
	_, _, warning := ParseLenient("maybe")
	assert.Contains(t, warning, `"maybe"`)
	assert.Contains(t, warning, "treating it as true")
}

// TestParseLenientMatchesEnv keeps ParseLenient in step with the installer's own
// reading of the variable. For every value that is set, the result must match what
// env.FromEnv reports for the same environment.
func TestParseLenientMatchesEnv(t *testing.T) {
	for _, v := range parseInputs {
		t.Run("'"+v+"'", func(t *testing.T) {
			enabled, set, _ := ParseLenient(v)
			if !set {
				return
			}
			t.Setenv("DD_PROCESS_MANAGER_ENABLED", v)
			assert.Equal(t, env.FromEnv().ProcessManagerEnabled, enabled)
		})
	}
}
