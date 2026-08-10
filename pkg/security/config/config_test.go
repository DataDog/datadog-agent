// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config holds config related files
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestHostDumpPullsUpPrerequisites(t *testing.T) {
	// host_dump on with the prerequisites off: they should be pulled up
	c := &RuntimeSecurityConfig{
		ActivityDumpHostDumpEnabled:     true,
		ActivityDumpEnabled:             false,
		ActivityDumpTraceSystemdCgroups: false,
		ActivityDumpTracedCgroupsCount:  5,
		SecurityProfileEnabled:          false,
	}

	assert.NoError(t, c.sanitizeRuntimeSecurityConfigActivityDump())
	assert.True(t, c.ActivityDumpEnabled, "activity dumps should be force-enabled")
	assert.True(t, c.ActivityDumpTraceSystemdCgroups, "systemd cgroup tracing should be force-enabled")
	assert.Equal(t, model.MaxTracedCgroupsCount, c.ActivityDumpTracedCgroupsCount, "traced cgroups count should be raised to the max")
}

func TestHostDumpNeverLowersOperatorValues(t *testing.T) {
	// operator already set a high count: upward-only means it is left untouched
	c := &RuntimeSecurityConfig{
		ActivityDumpHostDumpEnabled:     true,
		ActivityDumpEnabled:             true,
		ActivityDumpTraceSystemdCgroups: true,
		ActivityDumpTracedCgroupsCount:  model.MaxTracedCgroupsCount,
		SecurityProfileEnabled:          false,
	}

	assert.NoError(t, c.sanitizeRuntimeSecurityConfigActivityDump())
	assert.Equal(t, model.MaxTracedCgroupsCount, c.ActivityDumpTracedCgroupsCount)
}

func TestHostDumpDisabledIsNoop(t *testing.T) {
	// host_dump off: the prerequisites must not be touched
	c := &RuntimeSecurityConfig{
		ActivityDumpHostDumpEnabled:     false,
		ActivityDumpEnabled:             false,
		ActivityDumpTraceSystemdCgroups: false,
		ActivityDumpTracedCgroupsCount:  5,
		SecurityProfileEnabled:          false,
	}

	assert.NoError(t, c.sanitizeRuntimeSecurityConfigActivityDump())
	assert.False(t, c.ActivityDumpTraceSystemdCgroups)
	assert.Equal(t, 5, c.ActivityDumpTracedCgroupsCount)
}
