// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package impl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/validator/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type mockOption struct {
	hasValue bool
}

func (m mockOption) HasValue() bool {
	return m.hasValue
}

func TestValidateDependencies(t *testing.T) {
	tests := []struct {
		name          string
		logsEnabled   bool
		logEnabled    bool
		options       []def.Option
		expectError   bool
		errorContains string
	}{
		{
			name:        "validation passes",
			logsEnabled: true,
			logEnabled:  true,
			options:     []def.Option{mockOption{true}, mockOption{true}},
			expectError: false,
		},
		{
			name:          "logs_enabled false",
			logsEnabled:   false,
			logEnabled:    true,
			expectError:   true,
			errorContains: "logs are disabled",
		},
		{
			name:          "log_enabled false",
			logsEnabled:   true,
			logEnabled:    false,
			expectError:   true,
			errorContains: "logs are disabled",
		},
		{
			name:          "dependency missing",
			logsEnabled:   true,
			logEnabled:    true,
			options:       []def.Option{mockOption{false}},
			expectError:   true,
			errorContains: "dependency 0 is not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := config.NewMock(t)
			mockConfig.SetWithoutSource("logs_enabled", tt.logsEnabled)
			mockConfig.SetWithoutSource("log_enabled", tt.logEnabled)

			validator := &Validator{
				config: mockConfig,
				log:    logmock.New(t),
			}

			err := validator.ValidateDependencies(tt.options...)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGenOption(t *testing.T) {
	type testComponent struct {
		value string
	}

	type testDeps struct {
		Dep option.Option[string]
	}

	constructor := func(deps testDeps) testComponent {
		v, _ := deps.Dep.Get()
		return testComponent{value: v}
	}

	tests := []struct {
		name          string
		logsEnabled   bool
		dep           option.Option[string]
		checkDep      bool
		expectPresent bool
	}{
		{
			name:          "validation passes",
			logsEnabled:   true,
			dep:           option.New("test"),
			checkDep:      true,
			expectPresent: true,
		},
		{
			name:          "logs disabled",
			logsEnabled:   false,
			dep:           option.New("test"),
			expectPresent: false,
		},
		{
			name:          "dependency missing and checked",
			logsEnabled:   true,
			dep:           option.None[string](),
			checkDep:      true,
			expectPresent: false,
		},
		{
			name:          "dependency missing but not checked",
			logsEnabled:   true,
			dep:           option.None[string](),
			checkDep:      false,
			expectPresent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := config.NewMock(t)
			mockConfig.SetWithoutSource("logs_enabled", tt.logsEnabled)
			mockConfig.SetWithoutSource("log_enabled", true)

			validator := &Validator{
				config: mockConfig,
				log:    logmock.New(t),
			}

			deps := testDeps{Dep: tt.dep}

			var result option.Option[testComponent]
			if tt.checkDep {
				result = def.GenOption(validator, deps, constructor, &tt.dep)
			} else {
				result = def.GenOption(validator, deps, constructor)
			}

			_, ok := result.Get()
			assert.Equal(t, tt.expectPresent, ok)
		})
	}
}

func TestNewProvides(t *testing.T) {
	mockConfig := config.NewMock(t)
	mockConfig.SetWithoutSource("logs_enabled", true)
	mockConfig.SetWithoutSource("log_enabled", true)

	provides := NewProvides(Dependencies{
		Config: mockConfig,
		Log:    logmock.New(t),
	})

	assert.NotNil(t, provides.Validator)
	assert.NoError(t, provides.Validator.ValidateDependencies())
}
