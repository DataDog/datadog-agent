// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package run

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	systemprobeconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

func TestShouldExecSystemProbeLite_UseSystemProbeLiteDisabled(t *testing.T) {
	sysprobeConfig := sysprobeconfigimpl.NewMock(t)
	// use_system_probe_lite defaults to false
	cfg := &sysconfigtypes.Config{
		Enabled:        true,
		EnabledModules: map[sysconfigtypes.ModuleName]struct{}{systemprobeconfig.DiscoveryModule: {}},
	}
	assert.False(t, shouldExecSystemProbeLite(sysprobeConfig, cfg))
}

func TestShouldExecSystemProbeLite_OnlyDiscoveryEnabled(t *testing.T) {
	sysprobeConfig := sysprobeconfigimpl.NewMock(t)
	sysprobeConfig.SetWithoutSource("discovery.use_system_probe_lite", true)

	cfg := &sysconfigtypes.Config{
		Enabled:        true,
		EnabledModules: map[sysconfigtypes.ModuleName]struct{}{systemprobeconfig.DiscoveryModule: {}},
	}
	assert.True(t, shouldExecSystemProbeLite(sysprobeConfig, cfg))
}

func TestShouldExecSystemProbeLite_NoModulesEnabled(t *testing.T) {
	sysprobeConfig := sysprobeconfigimpl.NewMock(t)
	sysprobeConfig.SetWithoutSource("discovery.use_system_probe_lite", true)

	cfg := &sysconfigtypes.Config{
		Enabled:        false,
		EnabledModules: map[sysconfigtypes.ModuleName]struct{}{},
	}
	assert.True(t, shouldExecSystemProbeLite(sysprobeConfig, cfg))
}

func TestShouldExecSystemProbeLite_MultipleModulesEnabled(t *testing.T) {
	sysprobeConfig := sysprobeconfigimpl.NewMock(t)
	sysprobeConfig.SetWithoutSource("discovery.use_system_probe_lite", true)

	cfg := &sysconfigtypes.Config{
		Enabled: true,
		EnabledModules: map[sysconfigtypes.ModuleName]struct{}{
			systemprobeconfig.DiscoveryModule:     {},
			systemprobeconfig.NetworkTracerModule: {},
		},
	}
	assert.False(t, shouldExecSystemProbeLite(sysprobeConfig, cfg))
}

func TestShouldExecSystemProbeLite_OnlyNonDiscoveryModuleEnabled(t *testing.T) {
	sysprobeConfig := sysprobeconfigimpl.NewMock(t)
	sysprobeConfig.SetWithoutSource("discovery.use_system_probe_lite", true)

	cfg := &sysconfigtypes.Config{
		Enabled: true,
		EnabledModules: map[sysconfigtypes.ModuleName]struct{}{
			systemprobeconfig.NetworkTracerModule: {},
		},
	}
	assert.False(t, shouldExecSystemProbeLite(sysprobeConfig, cfg))
}

func TestShouldExecSystemProbeLite_ExternalSystemProbe(t *testing.T) {
	sysprobeConfig := sysprobeconfigimpl.NewMock(t)
	sysprobeConfig.SetWithoutSource("discovery.use_system_probe_lite", true)

	cfg := &sysconfigtypes.Config{
		Enabled:             true,
		EnabledModules:      map[sysconfigtypes.ModuleName]struct{}{systemprobeconfig.DiscoveryModule: {}},
		ExternalSystemProbe: true,
	}
	assert.False(t, shouldExecSystemProbeLite(sysprobeConfig, cfg))
}

func TestShouldExecSystemProbeLite_DiscoveryExplicitlyDisabled(t *testing.T) {
	sysprobeConfig := sysprobeconfigimpl.NewMock(t)
	sysprobeConfig.SetWithoutSource("discovery.use_system_probe_lite", true)
	sysprobeConfig.SetWithoutSource("discovery.enabled", false)

	cfg := &sysconfigtypes.Config{
		Enabled:        false,
		EnabledModules: map[sysconfigtypes.ModuleName]struct{}{},
	}
	// Discovery explicitly disabled with nothing else enabled -> exit cleanly, don't exec system-probe-lite
	assert.False(t, shouldExecSystemProbeLite(sysprobeConfig, cfg))
}

func TestShouldExecSystemProbeLite_DiscoveryExplicitlyEnabled(t *testing.T) {
	sysprobeConfig := sysprobeconfigimpl.NewMock(t)
	sysprobeConfig.SetWithoutSource("discovery.use_system_probe_lite", true)
	sysprobeConfig.SetWithoutSource("discovery.enabled", true)

	cfg := &sysconfigtypes.Config{
		Enabled:        true,
		EnabledModules: map[sysconfigtypes.ModuleName]struct{}{systemprobeconfig.DiscoveryModule: {}},
	}
	assert.True(t, shouldExecSystemProbeLite(sysprobeConfig, cfg))
}
