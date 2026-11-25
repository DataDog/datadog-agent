// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package module

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
)

func newProfilingConfig(runtimeEnabled bool) *config.Config {
	return &config.Config{
		Probe: &pconfig.Config{NetworkEnabled: true, EventStreamUseSyscallTaskStorage: true},
		RuntimeSecurity: &config.RuntimeSecurityConfig{
			RuntimeEnabled:           runtimeEnabled,
			ActivityDumpEnabled:      true,
			SecurityProfileEnabled:   true,
			SecurityProfileV2Enabled: true,
			SysCtlEnabled:            true,
		},
	}
}

// The event monitor also runs for other consumers, such as network process events, when CWS is off.
// It must not attach CWS profiling probes in that case.
func TestUpdateEventMonitorOptsDisablesProfilingWithoutRuntimeSecurity(t *testing.T) {
	configmock.NewSystemProbe(t)
	cfg := newProfilingConfig(false)
	UpdateEventMonitorOpts(&eventmonitor.Opts{}, cfg)

	assert.False(t, cfg.RuntimeSecurity.ActivityDumpEnabled)
	assert.False(t, cfg.RuntimeSecurity.SecurityProfileEnabled)
	assert.False(t, cfg.RuntimeSecurity.SecurityProfileV2Enabled)
	assert.False(t, cfg.RuntimeSecurity.SysCtlEnabled)
	assert.False(t, cfg.Probe.NetworkEnabled)
	assert.False(t, cfg.Probe.EventStreamUseSyscallTaskStorage)
}

func TestUpdateEventMonitorOptsKeepsProfilingWithRuntimeSecurity(t *testing.T) {
	configmock.NewSystemProbe(t)
	cfg := newProfilingConfig(true)
	UpdateEventMonitorOpts(&eventmonitor.Opts{}, cfg)

	assert.True(t, cfg.RuntimeSecurity.ActivityDumpEnabled)
	assert.True(t, cfg.RuntimeSecurity.SecurityProfileEnabled)
	assert.True(t, cfg.RuntimeSecurity.SecurityProfileV2Enabled)
	assert.True(t, cfg.Probe.EventStreamUseSyscallTaskStorage)
}

func TestUpdateEventMonitorOptsKeepsExplicitSyscallTaskStorage(t *testing.T) {
	for _, key := range []string{
		"event_monitoring_config.event_stream.use_syscall_task_storage",
		"runtime_security_config.event_stream.use_syscall_task_storage",
	} {
		t.Run(key, func(t *testing.T) {
			configmock.NewSystemProbe(t).SetInTest(key, true)
			cfg := newProfilingConfig(false)
			UpdateEventMonitorOpts(&eventmonitor.Opts{}, cfg)

			assert.True(t, cfg.Probe.EventStreamUseSyscallTaskStorage)
		})
	}
}
