// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParActionsAllowlist_ExplicitEnv(t *testing.T) {
	explicit := "com.datadoghq.http.request,com.datadoghq.http.response"
	got := parActionsAllowlist(explicit, "linux", true)
	assert.Equal(t, []string{"com.datadoghq.http.request", "com.datadoghq.http.response"}, got)
}

func TestParActionsAllowlist_ExplicitEnvOnReinstall(t *testing.T) {
	// Explicit env var always wins, even on reinstall.
	got := parActionsAllowlist("com.datadoghq.http.request", "windows", false)
	assert.Equal(t, []string{"com.datadoghq.http.request"}, got)
}

func TestParActionsAllowlist_DefaultNixFreshInstall(t *testing.T) {
	got := parActionsAllowlist("", "linux", true)
	assert.Equal(t, []string{parDefaultAllowlistNix}, got)
}

func TestParActionsAllowlist_DefaultWindowsFreshInstall(t *testing.T) {
	got := parActionsAllowlist("", "windows", true)
	assert.Equal(t, []string{parDefaultAllowlistWindows}, got)
}

func TestParActionsAllowlist_NoOverwriteOnReinstall(t *testing.T) {
	// No env var + reinstall → nil so WriteConfigs does not clobber existing allowlist.
	got := parActionsAllowlist("", "linux", false)
	assert.Nil(t, got)

	got = parActionsAllowlist("", "windows", false)
	assert.Nil(t, got)
}

func TestParActionsAllowlist_DefaultCurrentOSFreshInstall(t *testing.T) {
	// Current OS gets one of the two known defaults on fresh install.
	got := parActionsAllowlist("", runtime.GOOS, true)
	assert.Len(t, got, 1)
	assert.Contains(t,
		[]string{parDefaultAllowlistNix, parDefaultAllowlistWindows},
		got[0],
	)
}
