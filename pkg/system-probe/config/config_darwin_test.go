// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestNotableEventsModuleEnablement verifies macOS configuration controls module startup.
func TestNotableEventsModuleEnablement(t *testing.T) {
	for _, test := range []struct {
		name    string
		enabled bool
	}{
		{name: "disabled"},
		{name: "enabled", enabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			coreConfig := mock.New(t)
			mock.NewSystemProbe(t)
			coreConfig.SetInTest("notable_events.enabled", test.enabled)

			cfg, err := New("/doesnotexist", "")
			require.NoError(t, err)
			assert.Equal(t, test.enabled, cfg.ModuleIsEnabled(NotableEventsModule))
		})
	}
}

// TestDefaultConfigDirIsTheAgentConfPath verifies that, with no path given, system-probe.yaml is
// looked up next to datadog.yaml rather than in the Linux /etc/datadog-agent.
func TestDefaultConfigDirIsTheAgentConfPath(t *testing.T) {
	assert.Equal(t, "/opt/datadog-agent/etc", defaultConfigDir)
}
