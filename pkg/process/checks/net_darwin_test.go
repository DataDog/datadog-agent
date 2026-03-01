// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// syscfgWithNPM returns a minimal sysconfigtypes.Config with the NetworkTracer
// module enabled (or not), mirroring what system-probe would hand to the check.
func syscfgWithNPM(npmEnabled bool) *sysconfigtypes.Config {
	cfg := &sysconfigtypes.Config{
		Enabled:        true,
		EnabledModules: make(map[sysconfigtypes.ModuleName]struct{}),
	}
	if npmEnabled {
		cfg.EnabledModules[sysconfig.NetworkTracerModule] = struct{}{}
	}
	return cfg
}

// TestConnectionsCheck_IsEnabled_DarwinGuard exercises the CB-3 fix:
// on Darwin the check must be disabled unless network_config.enabled is
// explicitly true, regardless of the other conditions being satisfied.
func TestConnectionsCheck_IsEnabled_DarwinGuard(t *testing.T) {
	tests := []struct {
		name            string
		networkEnabled  bool // network_config.enabled in sysprobe yaml
		npmModule       bool // NetworkTracerModule present in syscfg
		syscfgEnabled   bool // syscfg.Enabled
		directSend      bool // network_config.direct_send
		agentFlavor     string
		expectedEnabled bool
	}{
		{
			name:            "darwin guard blocks when network_config.enabled is false",
			networkEnabled:  false,
			npmModule:       true,
			syscfgEnabled:   true,
			directSend:      false,
			agentFlavor:     flavor.ProcessAgent,
			expectedEnabled: false,
		},
		{
			name:            "enabled when all conditions met",
			networkEnabled:  true,
			npmModule:       true,
			syscfgEnabled:   true,
			directSend:      false,
			agentFlavor:     flavor.ProcessAgent,
			expectedEnabled: true,
		},
		{
			name:            "disabled when not process-agent flavor",
			networkEnabled:  true,
			npmModule:       true,
			syscfgEnabled:   true,
			directSend:      false,
			agentFlavor:     flavor.DefaultAgent,
			expectedEnabled: false,
		},
		{
			name:            "disabled when NPM module not enabled",
			networkEnabled:  true,
			npmModule:       false,
			syscfgEnabled:   true,
			directSend:      false,
			agentFlavor:     flavor.ProcessAgent,
			expectedEnabled: false,
		},
		{
			name:            "disabled when syscfg.Enabled is false",
			networkEnabled:  true,
			npmModule:       true,
			syscfgEnabled:   false,
			directSend:      false,
			agentFlavor:     flavor.ProcessAgent,
			expectedEnabled: false,
		},
		{
			name:            "disabled when direct_send is true",
			networkEnabled:  true,
			npmModule:       true,
			syscfgEnabled:   true,
			directSend:      true,
			agentFlavor:     flavor.ProcessAgent,
			expectedEnabled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Restore flavor after each subtest.
			origFlavor := flavor.GetFlavor()
			defer flavor.SetFlavor(origFlavor)
			flavor.SetFlavor(tc.agentFlavor)

			sysCfg := syscfgWithNPM(tc.npmModule)
			sysCfg.Enabled = tc.syscfgEnabled

			sysprobeYaml := configmock.NewSystemProbe(t)
			sysprobeYaml.SetWithoutSource("network_config.enabled", tc.networkEnabled)
			sysprobeYaml.SetWithoutSource("network_config.direct_send", tc.directSend)

			check := &ConnectionsCheck{
				syscfg:             sysCfg,
				sysprobeYamlConfig: sysprobeYaml,
			}
			assert.Equal(t, tc.expectedEnabled, check.IsEnabled(), tc.name)
		})
	}
}
