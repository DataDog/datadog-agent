// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupComponents_EnvUnsetInitMode_HandleAlive_NoForwarder(t *testing.T) {
	got, err := setupComponents(setupInput{})
	require.NoError(t, err)
	assert.Nil(t, got.Forwarder)
	require.NotNil(t, got.Child) // production handle, mutable by mode
	assert.Equal(t, ChildHandle(got.Child), got.Handle)
	assert.Equal(t, DefaultPort, got.Port)
}

func TestSetupComponents_EnvSetInitMode_ForwarderEnabled(t *testing.T) {
	got, err := setupComponents(setupInput{userAppPort: "8080"})
	require.NoError(t, err)
	require.NotNil(t, got.Forwarder)
	require.NotNil(t, got.Child)
}

func TestSetupComponents_EnvSetSidecarMode_ForwarderDisabledWithWarn(t *testing.T) {
	got, err := setupComponents(setupInput{userAppPort: "8080", sidecarMode: true})
	require.NoError(t, err)
	assert.Nil(t, got.Forwarder, "sidecar mode must disable the forwarder")
	assert.Nil(t, got.Child, "sidecar mode uses noop child")
	require.NotNil(t, got.Handle) // noop handle still provided
}

func TestSetupComponents_EnvUnsetSidecarMode_NoForwarderNoopHandle(t *testing.T) {
	got, err := setupComponents(setupInput{sidecarMode: true})
	require.NoError(t, err)
	assert.Nil(t, got.Forwarder)
	assert.Nil(t, got.Child)
	require.NotNil(t, got.Handle)
}

func TestSetupComponents_EnvInvalid_ReturnsError(t *testing.T) {
	for _, raw := range []string{"abc", "0", "65536", "9000"} {
		_, err := setupComponents(setupInput{userAppPort: raw})
		assert.Error(t, err, "raw=%q should fail", raw)
	}
}

// In sidecar mode userAppPort is only used for a warning log — the forwarder
// is never built — so a stale or colliding value (e.g. inherited from an
// init-mode config) must not fail setup the way it does in init mode above.
func TestSetupComponents_SidecarMode_InvalidUserAppPort_NoError(t *testing.T) {
	for _, raw := range []string{"abc", "0", "65536", "9000"} {
		got, err := setupComponents(setupInput{userAppPort: raw, sidecarMode: true})
		require.NoError(t, err, "raw=%q must not fail setup in sidecar mode", raw)
		assert.Nil(t, got.Forwarder)
		assert.Nil(t, got.Child)
		require.NotNil(t, got.Handle)
	}
}

func TestSetupComponents_CustomLifecyclePort(t *testing.T) {
	got, err := setupComponents(setupInput{lifecyclePort: "9001"})
	require.NoError(t, err)
	assert.Equal(t, 9001, got.Port)
}

func TestSetupComponents_InvalidLifecyclePort_ReturnsError(t *testing.T) {
	for _, raw := range []string{"abc", "0", "65536", "-1"} {
		_, err := setupComponents(setupInput{lifecyclePort: raw})
		assert.Error(t, err, "lifecycle port %q should fail", raw)
	}
}

// Unlike userAppPort, lifecyclePort is parsed before the sidecar early
// return and is used regardless of mode (it sets SetupComponents.Port), so
// it must still be validated in sidecar mode.
func TestSetupComponents_SidecarMode_InvalidLifecyclePort_StillReturnsError(t *testing.T) {
	for _, raw := range []string{"abc", "0", "65536", "-1"} {
		_, err := setupComponents(setupInput{lifecyclePort: raw, sidecarMode: true})
		assert.Error(t, err, "lifecycle port %q should fail even in sidecar mode", raw)
	}
}

func TestSetupComponents_UserAppPortCollidesWithCustomLifecyclePort_ReturnsError(t *testing.T) {
	// Both ports set to 9001 — must be rejected regardless of the default (9000).
	_, err := setupComponents(setupInput{lifecyclePort: "9001", userAppPort: "9001"})
	assert.Error(t, err, "user-app port equal to a non-default lifecycle port must be rejected")
}

func TestSetupComponents_UserAppPortOnDefaultWhenLifecycleMoved_IsAccepted(t *testing.T) {
	// Lifecycle moved to 9001; user app on 9000 is now free.
	got, err := setupComponents(setupInput{lifecyclePort: "9001", userAppPort: "9000"})
	require.NoError(t, err)
	assert.Equal(t, 9001, got.Port)
	require.NotNil(t, got.Forwarder)
}

func TestSetupComponents_CustomTimeouts(t *testing.T) {
	got, err := setupComponents(setupInput{userAppPort: "8080", forwardMs: "5000", readyMs: "10000", validateMs: "15000"})
	require.NoError(t, err)
	require.NotNil(t, got.Forwarder)
	assert.Equal(t, 5*time.Second, got.Forwarder.forwardTimeout)
	assert.Equal(t, 10*time.Second, got.Forwarder.readyTimeout)
	assert.Equal(t, 15*time.Second, got.Forwarder.validateTimeout)
}

func TestSetupComponents_InvalidTimeoutMs_ReturnsError(t *testing.T) {
	for _, raw := range []string{"abc", "0", "-1"} {
		_, err := setupComponents(setupInput{userAppPort: "8080", forwardMs: raw})
		assert.Error(t, err, "forward timeout %q should fail", raw)
	}
}

// Timeouts are only used to construct the Forwarder, which sidecar mode never
// builds, so an invalid timeout inherited from an init-mode config must not
// fail setup in sidecar mode either.
func TestSetupComponents_SidecarMode_InvalidTimeoutMs_NoError(t *testing.T) {
	for _, raw := range []string{"abc", "0", "-1"} {
		got, err := setupComponents(setupInput{userAppPort: "8080", forwardMs: raw, sidecarMode: true})
		require.NoError(t, err, "forward timeout %q must not fail setup in sidecar mode", raw)
		assert.Nil(t, got.Forwarder)
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
