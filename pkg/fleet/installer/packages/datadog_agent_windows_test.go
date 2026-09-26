// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

func TestPreStopExperimentRejectsInvalidMSI(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(fmt.Sprintf("corrupt=%t", corrupt), func(t *testing.T) {
			paths.SetupTestPaths(t)
			t.Setenv("DD_FIPS_MODE", "true")
			if corrupt {
				dir := filepath.Join(paths.PackagesPath, "datadog-agent", "stable")
				require.NoError(t, os.MkdirAll(dir, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog-fips-agent-7.83.2-1-x86_64.msi"), nil, 0600))
			}
			err := RunHook(HookContext{
				Context: context.Background(),
				Package: "datadog-agent",
				Hook:    "preStopExperiment",
			})
			require.ErrorContains(t, err, "invalid rollback MSI")
		})
	}
}

// TestGetenvAgentUserKeepRightsFallback verifies getenv() falls back to the registry-stored
// DDAGENTUSER_KEEP_RIGHTS value when not provided on the command line.
func TestGetenvAgentUserKeepRightsFallback(t *testing.T) {
	tests := []struct {
		name               string
		envValue           string
		registryValue      string
		registryErr        error
		expectedKeepRights string
	}{
		{
			name:               "explicit env var takes precedence over registry",
			envValue:           "1",
			registryValue:      "0",
			expectedKeepRights: "1",
		},
		{
			name:               "falls back to registry when env var is unset",
			envValue:           "",
			registryValue:      "1",
			expectedKeepRights: "1",
		},
		{
			name:               "empty registry value leaves param empty",
			envValue:           "",
			registryValue:      "",
			expectedKeepRights: "",
		},
		{
			name:               "registry read error leaves param empty",
			envValue:           "",
			registryErr:        errors.New("registry unavailable"),
			expectedKeepRights: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DDAGENTUSER_KEEP_RIGHTS", tt.envValue)

			previous := getAgentUserKeepRightsFromRegistry
			getAgentUserKeepRightsFromRegistry = func() (string, error) {
				return tt.registryValue, tt.registryErr
			}
			t.Cleanup(func() { getAgentUserKeepRightsFromRegistry = previous })

			env := getenv()
			assert.Equal(t, tt.expectedKeepRights, env.MsiParams.AgentUserKeepRights)
		})
	}
}

// TestArgsHaveProperty verifies the guard installAgentPackage uses to detect an MSI property
// already supplied via install args, so a getenv() fallback doesn't override it.
func TestArgsHaveProperty(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		property string
		expected bool
	}{
		{
			name:     "property present",
			args:     []string{"FLEET_INSTALL=1", "DDAGENTUSER_KEEP_RIGHTS=0"},
			property: "DDAGENTUSER_KEEP_RIGHTS",
			expected: true,
		},
		{
			name:     "property absent",
			args:     []string{"FLEET_INSTALL=1"},
			property: "DDAGENTUSER_KEEP_RIGHTS",
			expected: false,
		},
		{
			name:     "no args",
			args:     nil,
			property: "DDAGENTUSER_KEEP_RIGHTS",
			expected: false,
		},
		{
			name:     "does not match on property name prefix alone",
			args:     []string{"DDAGENTUSER_KEEP_RIGHTS_EXTRA=1"},
			property: "DDAGENTUSER_KEEP_RIGHTS",
			expected: false,
		},
		{
			name:     "agent user name property present",
			args:     []string{"DDAGENTUSER_NAME=.\\customuser"},
			property: "DDAGENTUSER_NAME",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, argsHaveProperty(tt.args, tt.property))
		})
	}
}
