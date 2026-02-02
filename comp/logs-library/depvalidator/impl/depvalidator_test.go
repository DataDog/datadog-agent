// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package depvalidatorimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	depvalidatordef "github.com/DataDog/datadog-agent/comp/logs-library/depvalidator/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Test structs for validation
type testDepsAllSet struct {
	Dep1 option.Option[string]
	Dep2 option.Option[int]
}

type testDepsOneNone struct {
	Dep1 option.Option[string]
	Dep2 option.Option[int]
}

type testDepsWithOptionalTag struct {
	Required option.Option[string]
	Optional option.Option[string] `depvalidator:"optional"`
}

type testDepsMixed struct {
	RegularField string
	OptionalDep  option.Option[int]
}

type testDepsEmpty struct{}

// newTestComponent creates a test depvalidator component with the given logs_enabled setting
func newTestComponent(t *testing.T, logsEnabled bool) *Provides {
	cfg := configmock.NewMock(t)
	cfg.SetWithoutSource("logs_enabled", logsEnabled)
	provides := NewProvides(Dependencies{
		Config: cfg,
		Log:    logmock.New(t),
	})
	return &provides
}

func TestLogsEnabled(t *testing.T) {
	tests := []struct {
		name           string
		configOverride map[string]interface{}
		expected       bool
	}{
		{
			name:           "logs_enabled true",
			configOverride: map[string]interface{}{"logs_enabled": true},
			expected:       true,
		},
		{
			name:           "logs_enabled false",
			configOverride: map[string]interface{}{"logs_enabled": false},
			expected:       false,
		},
		{
			name:           "log_enabled true (deprecated)",
			configOverride: map[string]interface{}{"log_enabled": true},
			expected:       true,
		},
		{
			name:           "both false",
			configOverride: map[string]interface{}{"logs_enabled": false, "log_enabled": false},
			expected:       false,
		},
		{
			name:           "logs_enabled true overrides log_enabled false",
			configOverride: map[string]interface{}{"logs_enabled": true, "log_enabled": false},
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.NewMock(t)
			for k, v := range tt.configOverride {
				cfg.SetWithoutSource(k, v)
			}
			provides := NewProvides(Dependencies{
				Config: cfg,
				Log:    logmock.New(t),
			})
			assert.Equal(t, tt.expected, provides.Comp.LogsEnabled())
		})
	}
}

func TestValidateDependencies_AllSet(t *testing.T) {
	provides := newTestComponent(t, true)

	deps := testDepsAllSet{
		Dep1: option.New("value"),
		Dep2: option.New(42),
	}

	err := provides.Comp.ValidateDependencies(deps)
	assert.NoError(t, err)
}

func TestValidateDependencies_OneNone(t *testing.T) {
	provides := newTestComponent(t, true)

	deps := testDepsOneNone{
		Dep1: option.New("value"),
		Dep2: option.None[int](),
	}

	err := provides.Comp.ValidateDependencies(deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Dep2")
	assert.Contains(t, err.Error(), "not set")
}

func TestValidateDependencies_FirstNone(t *testing.T) {
	provides := newTestComponent(t, true)

	deps := testDepsOneNone{
		Dep1: option.None[string](),
		Dep2: option.New(42),
	}

	err := provides.Comp.ValidateDependencies(deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Dep1")
}

func TestValidateDependencies_WithOptionalTag(t *testing.T) {
	provides := newTestComponent(t, true)

	// Optional field is None but should be skipped due to tag
	deps := testDepsWithOptionalTag{
		Required: option.New("value"),
		Optional: option.None[string](),
	}

	err := provides.Comp.ValidateDependencies(deps)
	assert.NoError(t, err)
}

func TestValidateDependencies_OptionalTagRequiredMissing(t *testing.T) {
	provides := newTestComponent(t, true)

	// Required field is None - should fail
	deps := testDepsWithOptionalTag{
		Required: option.None[string](),
		Optional: option.None[string](),
	}

	err := provides.Comp.ValidateDependencies(deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Required")
}

func TestValidateDependencies_MixedFields(t *testing.T) {
	provides := newTestComponent(t, true)

	// Non-option fields should be ignored
	deps := testDepsMixed{
		RegularField: "some value",
		OptionalDep:  option.New(123),
	}

	err := provides.Comp.ValidateDependencies(deps)
	assert.NoError(t, err)
}

func TestValidateDependencies_MixedFieldsWithNone(t *testing.T) {
	provides := newTestComponent(t, true)

	deps := testDepsMixed{
		RegularField: "some value",
		OptionalDep:  option.None[int](),
	}

	err := provides.Comp.ValidateDependencies(deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OptionalDep")
}

func TestValidateDependencies_EmptyStruct(t *testing.T) {
	provides := newTestComponent(t, true)

	deps := testDepsEmpty{}

	err := provides.Comp.ValidateDependencies(deps)
	assert.NoError(t, err)
}

func TestValidateDependencies_Pointer(t *testing.T) {
	provides := newTestComponent(t, true)

	deps := &testDepsAllSet{
		Dep1: option.New("value"),
		Dep2: option.New(42),
	}

	err := provides.Comp.ValidateDependencies(deps)
	assert.NoError(t, err)
}

func TestValidateDependencies_NilPointer(t *testing.T) {
	provides := newTestComponent(t, true)

	var deps *testDepsAllSet

	err := provides.Comp.ValidateDependencies(deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestValidateDependencies_NonStruct(t *testing.T) {
	provides := newTestComponent(t, true)

	err := provides.Comp.ValidateDependencies("not a struct")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a struct")
}

func TestValidateIfEnabled_LogsDisabled(t *testing.T) {
	provides := newTestComponent(t, false)

	// Even with None values, should return ErrLogsDisabled when logs disabled
	deps := testDepsOneNone{
		Dep1: option.None[string](),
		Dep2: option.None[int](),
	}

	err := provides.Comp.ValidateIfEnabled(deps)
	assert.ErrorIs(t, err, depvalidatordef.ErrLogsDisabled)
}

func TestValidateIfEnabled_LogsEnabledValid(t *testing.T) {
	provides := newTestComponent(t, true)

	deps := testDepsAllSet{
		Dep1: option.New("value"),
		Dep2: option.New(42),
	}

	err := provides.Comp.ValidateIfEnabled(deps)
	assert.NoError(t, err)
}

func TestValidateIfEnabled_LogsEnabledInvalid(t *testing.T) {
	provides := newTestComponent(t, true)

	deps := testDepsOneNone{
		Dep1: option.New("value"),
		Dep2: option.None[int](),
	}

	err := provides.Comp.ValidateIfEnabled(deps)
	require.Error(t, err)
	assert.NotErrorIs(t, err, depvalidatordef.ErrLogsDisabled)
	assert.Contains(t, err.Error(), "Dep2")
}
