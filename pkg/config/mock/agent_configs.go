// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package mock

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// InitAgentConfigs replaces the global datadog and system-probe configurations by new ones initialized exactly like
// the Agent does when it starts (setup.InitConfigObjects): unlike New and NewSystemProbe, the post-init fixups and
// override functions are registered and the schema is not dynamic. It's meant for tests that load real configuration
// files into the globals through the config components.
//
// The environment variables are read while the configurations are built, so they must be set before. The previous
// configurations and override functions are restored at the end of the test, and New and NewSystemProbe return the
// new configurations until then.
func InitAgentConfigs(t testing.TB) {
	m.Lock()
	defer m.Unlock()
	require.False(t, isConfigMocked || isSystemProbeConfigMocked, "the configurations are already mocked in this test")

	isConfigMocked, isSystemProbeConfigMocked = true, true
	originalDatadog := setup.GlobalConfigBuilder()
	originalSystemProbe := setup.GlobalSystemProbeConfigBuilder()
	model.RestoreOverrideFuncs(t)
	t.Cleanup(func() {
		m.Lock()
		defer m.Unlock()
		isConfigMocked, isSystemProbeConfigMocked = false, false
		setup.SetDatadog(originalDatadog)         // nolint: forbidigo // legitimate use of SetDatadog
		setup.SetSystemProbe(originalSystemProbe) // nolint: forbidigo // legitimate use of SetSystemProbe
	})

	setup.InitConfigObjects()
}
