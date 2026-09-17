// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && test

package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// processService returns the "process" (datadog-process-agent) subservice definition.
func processService(t *testing.T, core, sysprobe model.Reader) Servicedef {
	for _, s := range subservices(core, sysprobe) {
		if s.name == "process" {
			return s
		}
	}
	require.FailNow(t, "process service definition not found")
	return Servicedef{}
}

// disableCollection turns off every process/network collection setting that would otherwise start process-agent, so the GUI condition is what's under test.
func disableCollection(core, sysprobe model.BuildableConfig) {
	core.SetInTest("process_config.enabled", "disabled")
	core.SetInTest("process_config.process_collection.enabled", false)
	core.SetInTest("process_config.container_collection.enabled", false)
	core.SetInTest("process_config.process_discovery.enabled", false)
	sysprobe.SetInTest("network_config.enabled", false)
	sysprobe.SetInTest("system_probe_config.enabled", false)
}

// TestProcessServiceStartsForGUI verifies process-agent is started to serve the GUI's peer-identity API even when all process/network collection is disabled (CWE-214, see comp/core/gui/impl/peeridentity_windows.go).
func TestProcessServiceStartsForGUI(t *testing.T) {
	core := configmock.New(t)
	sysprobe := configmock.NewSystemProbe(t)
	disableCollection(core, sysprobe)
	core.SetInTest("GUI_port", "5002")

	svc := processService(t, core, sysprobe)
	assert.True(t, svc.IsEnabled())
}

// TestProcessServiceStoppedWhenGUIDisabled verifies that with both collection and the GUI disabled, process-agent is not started.
func TestProcessServiceStoppedWhenGUIDisabled(t *testing.T) {
	core := configmock.New(t)
	sysprobe := configmock.NewSystemProbe(t)
	disableCollection(core, sysprobe)
	core.SetInTest("GUI_port", "-1")

	svc := processService(t, core, sysprobe)
	assert.False(t, svc.IsEnabled())
}
