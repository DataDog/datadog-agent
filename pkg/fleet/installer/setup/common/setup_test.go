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
	got := parActionsAllowlist(explicit, "linux")
	assert.Equal(t, []string{"com.datadoghq.http.request", "com.datadoghq.http.response"}, got)
}

func TestParActionsAllowlist_ExplicitEnvWindows(t *testing.T) {
	explicit := "com.datadoghq.http.request"
	got := parActionsAllowlist(explicit, "windows")
	assert.Equal(t, []string{"com.datadoghq.http.request"}, got)
}

func TestParActionsAllowlist_DefaultNix(t *testing.T) {
	got := parActionsAllowlist("", "linux")
	assert.Equal(t, []string{parDefaultAllowlistNix}, got)
}

func TestParActionsAllowlist_DefaultWindows(t *testing.T) {
	got := parActionsAllowlist("", "windows")
	assert.Equal(t, []string{parDefaultAllowlistWindows}, got)
}

func TestParActionsAllowlist_DefaultCurrentOS(t *testing.T) {
	// Verify that the current OS gets a sensible default (not an empty list).
	got := parActionsAllowlist("", runtime.GOOS)
	assert.Len(t, got, 1)
	assert.Contains(t,
		[]string{parDefaultAllowlistNix, parDefaultAllowlistWindows},
		got[0],
	)
}
