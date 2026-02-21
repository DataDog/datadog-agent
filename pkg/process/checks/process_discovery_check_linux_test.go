// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package checks

import (
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/stretchr/testify/assert"
)

func TestProcessDiscoveryLinuxWithRunInCoreAgent(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)

	// Ensure the process discovery checks run on the core agent only when run in core agent mode is enabled
	cfg := configmock.New(t)
	sysCfg := configmock.NewSystemProbe(t)
	cfg.SetInTest("process_config.process_collection.enabled", false)
	cfg.SetInTest("process_config.process_discovery.enabled", true)
	cfg.SetInTest("process_config.run_in_core_agent.enabled", true)

	tests := []struct {
		name    string
		flavor  string
		enabled bool
	}{
		{
			name:    "enabled on the core agent",
			flavor:  flavor.DefaultAgent,
			enabled: true,
		},
		{
			name:    "disabled on the process agent",
			flavor:  flavor.ProcessAgent,
			enabled: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flavor.SetFlavor(tc.flavor)
			check := NewProcessDiscoveryCheck(cfg, sysCfg)
			assert.Equal(t, tc.enabled, check.IsEnabled())
		})
	}
}
