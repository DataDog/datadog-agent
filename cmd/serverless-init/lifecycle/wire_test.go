// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupComponents_EnvUnsetInitMode_HandleAlive_NoForwarder(t *testing.T) {
	got, err := setupComponents("", false /*sidecar*/)
	require.NoError(t, err)
	assert.Nil(t, got.Forwarder)
	require.NotNil(t, got.Child) // production handle, mutable by mode
	assert.Equal(t, ChildHandle(got.Child), got.Handle)
}

func TestSetupComponents_EnvSetInitMode_ForwarderEnabled(t *testing.T) {
	got, err := setupComponents("8080", false)
	require.NoError(t, err)
	require.NotNil(t, got.Forwarder)
	require.NotNil(t, got.Child)
}

func TestSetupComponents_EnvSetSidecarMode_ForwarderDisabledWithWarn(t *testing.T) {
	got, err := setupComponents("8080", true)
	require.NoError(t, err)
	assert.Nil(t, got.Forwarder, "sidecar mode must disable the forwarder")
	assert.Nil(t, got.Child, "sidecar mode uses noop child")
	require.NotNil(t, got.Handle) // noop handle still provided
}

func TestSetupComponents_EnvUnsetSidecarMode_NoForwarderNoopHandle(t *testing.T) {
	got, err := setupComponents("", true)
	require.NoError(t, err)
	assert.Nil(t, got.Forwarder)
	assert.Nil(t, got.Child)
	require.NotNil(t, got.Handle)
}

func TestSetupComponents_EnvInvalid_ReturnsError(t *testing.T) {
	for _, raw := range []string{"abc", "0", "65536", "9000"} {
		_, err := setupComponents(raw, false)
		assert.Error(t, err, "raw=%q should fail", raw)
	}
}

// SetupFromEnv must read UserAppPortEnvVar from the process environment
// and delegate to setupComponents. Pins the env binding so a refactor
// can't silently swap the env-var name or skip the lookup.
func TestSetupFromEnv_ReadsEnvVar_BuildsForwarder(t *testing.T) {
	t.Setenv(UserAppPortEnvVar, "8080")
	got, err := SetupFromEnv(false /*sidecar*/)
	require.NoError(t, err)
	require.NotNil(t, got.Forwarder)
	require.NotNil(t, got.Child)
}

func TestSetupFromEnv_UnsetEnv_NoForwarder(t *testing.T) {
	t.Setenv(UserAppPortEnvVar, "")
	got, err := SetupFromEnv(false)
	require.NoError(t, err)
	assert.Nil(t, got.Forwarder)
}

func TestSetupFromEnv_PropagatesParseError(t *testing.T) {
	t.Setenv(UserAppPortEnvVar, "abc")
	_, err := SetupFromEnv(false)
	require.Error(t, err)
}

// TestSetupFallback_InitMode verifies that SetupFallback returns a no-forwarder
// init-mode setup regardless of the env var's value. Called by MicroVM.Init
// when SetupFromEnv fails (invalid port) so the lifecycle server still starts
// on port 9000 and the platform can complete handshakes.
//
// SetupFallback calls setupComponents with an empty port, which always
// succeeds (see TestParseUserAppPort_UnsetReturnsZeroNoError). The panic
// branch is therefore unreachable and not exercised here.
func TestSetupFallback_InitMode(t *testing.T) {
	t.Setenv(UserAppPortEnvVar, "abc") // invalid — must be ignored by SetupFallback
	got := SetupFallback(false /*sidecar*/)
	assert.Nil(t, got.Forwarder, "fallback must have no forwarder")
	require.NotNil(t, got.Child, "fallback init mode must provide a Child for RunInit")
	assert.Equal(t, ChildHandle(got.Child), got.Handle)
}

// TestSetupFallback_SidecarMode verifies the noop-handle path. Like
// TestSetupFallback_InitMode, the panic branch is unreachable because
// SetupFallback always passes an empty port to setupComponents.
func TestSetupFallback_SidecarMode(t *testing.T) {
	t.Setenv(UserAppPortEnvVar, "abc") // invalid — must be ignored by SetupFallback
	got := SetupFallback(true /*sidecar*/)
	assert.Nil(t, got.Forwarder)
	assert.Nil(t, got.Child)
	require.NotNil(t, got.Handle)
}
