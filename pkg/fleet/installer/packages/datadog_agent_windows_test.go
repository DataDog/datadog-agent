// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetenvAgentUserKeepRightsFallback verifies that getenv() falls back to the registry-stored
// DDAGENTUSER_KEEP_RIGHTS value when it isn't provided on the command line, which is what allows
// fleet upgrades (which uninstall then reinstall the MSI as two separate transactions and don't
// pass the flag themselves) to carry the operator's opt-out forward.
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
