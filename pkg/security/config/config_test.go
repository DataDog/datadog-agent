// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config holds config related files
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// the host capture window is a V2 feature: enabling it must pull up the V2 manager and the
// security profile manager, upward-only.
func TestHostDumpPullsUpV2Prerequisites(t *testing.T) {
	c := &RuntimeSecurityConfig{
		SecurityProfileV2HostDumpEnabled:  true,
		RuntimeEnabled:                    false,
		SecurityProfileEnabled:            false,
		SecurityProfileV2Enabled:          false,
		ActivityDumpLocalStorageDirectory: "/tmp/captures",
		SecurityProfileDir:                "/opt/datadog-agent/run/runtime-security/profiles",
	}

	applyHostDumpPrerequisites(c)
	// without this, UpdateEventMonitorOpts would call DisableRuntimeSecurity and turn the
	// security profile manager back off, so the capture commands would never work
	assert.True(t, c.RuntimeEnabled, "runtime security should be force-enabled")
	assert.True(t, c.IsRuntimeEnabled(), "the CWS command server gate should be satisfied")
	assert.True(t, c.SecurityProfileEnabled, "security profiles should be force-enabled")
	assert.True(t, c.SecurityProfileV2Enabled, "the V2 manager should be force-enabled")
	assert.Equal(t, "/tmp/captures", c.SecurityProfileDir, "profile dir should be aligned with the dump output directory")
}

func TestHostDumpDisabledIsNoop(t *testing.T) {
	c := &RuntimeSecurityConfig{
		SecurityProfileV2HostDumpEnabled:  false,
		RuntimeEnabled:                    false,
		SecurityProfileEnabled:            false,
		SecurityProfileV2Enabled:          false,
		ActivityDumpLocalStorageDirectory: "/tmp/captures",
		SecurityProfileDir:                "/opt/datadog-agent/run/runtime-security/profiles",
	}

	applyHostDumpPrerequisites(c)
	assert.False(t, c.RuntimeEnabled)
	assert.False(t, c.SecurityProfileEnabled)
	assert.False(t, c.SecurityProfileV2Enabled)
	assert.Equal(t, "/opt/datadog-agent/run/runtime-security/profiles", c.SecurityProfileDir)
}
