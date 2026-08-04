// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- parsePort: user-app-port semantics (defaultVal=0, forbidden=DefaultPort) ---

func TestParsePort_UserApp_UnsetReturnsZeroNoError(t *testing.T) {
	port, err := parsePort(UserAppPortEnvVar, "", 0, DefaultPort)
	require.NoError(t, err)
	assert.Equal(t, 0, port) // 0 = "not set"
}

func TestParsePort_UserApp_ValidPort(t *testing.T) {
	port, err := parsePort(UserAppPortEnvVar, "8080", 0, DefaultPort)
	require.NoError(t, err)
	assert.Equal(t, 8080, port)
}

func TestParsePort_UserApp_AcceptsLowerBound(t *testing.T) {
	port, err := parsePort(UserAppPortEnvVar, "1", 0, DefaultPort)
	require.NoError(t, err)
	assert.Equal(t, 1, port)
}

func TestParsePort_UserApp_AcceptsUpperBound(t *testing.T) {
	port, err := parsePort(UserAppPortEnvVar, "65535", 0, DefaultPort)
	require.NoError(t, err)
	assert.Equal(t, 65535, port)
}

func TestParsePort_UserApp_RejectsNonNumeric(t *testing.T) {
	_, err := parsePort(UserAppPortEnvVar, "abc", 0, DefaultPort)
	require.Error(t, err)
	assert.ErrorContains(t, err, UserAppPortEnvVar)
	assert.ErrorContains(t, err, "abc")
}

func TestParsePort_UserApp_RejectsZero(t *testing.T) {
	_, err := parsePort(UserAppPortEnvVar, "0", 0, DefaultPort)
	require.Error(t, err)
	assert.ErrorContains(t, err, UserAppPortEnvVar)
	assert.ErrorContains(t, err, "0")
}

func TestParsePort_UserApp_RejectsAboveRange(t *testing.T) {
	_, err := parsePort(UserAppPortEnvVar, "65536", 0, DefaultPort)
	require.Error(t, err)
	assert.ErrorContains(t, err, UserAppPortEnvVar)
	assert.ErrorContains(t, err, "65536")
}

func TestParsePort_UserApp_RejectsNegative(t *testing.T) {
	_, err := parsePort(UserAppPortEnvVar, "-1", 0, DefaultPort)
	require.Error(t, err)
	assert.ErrorContains(t, err, UserAppPortEnvVar)
	assert.ErrorContains(t, err, "-1")
}

func TestParsePort_UserApp_RejectsForbiddenPort(t *testing.T) {
	_, err := parsePort(UserAppPortEnvVar, "9000", 0, DefaultPort)
	require.Error(t, err)
	assert.ErrorContains(t, err, UserAppPortEnvVar)
	assert.ErrorContains(t, err, "9000")
}

// --- parsePort: lifecycle-port semantics (defaultVal=DefaultPort, no forbidden) ---

func TestParsePort_Lifecycle_UnsetReturnsDefaultPort(t *testing.T) {
	port, err := parsePort(LifecyclePortEnvVar, "", DefaultPort)
	require.NoError(t, err)
	assert.Equal(t, DefaultPort, port)
}

func TestParsePort_Lifecycle_ValidPort(t *testing.T) {
	port, err := parsePort(LifecyclePortEnvVar, "9001", DefaultPort)
	require.NoError(t, err)
	assert.Equal(t, 9001, port)
}

func TestParsePort_Lifecycle_AcceptsBounds(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int
	}{
		{"1", 1}, {"65535", 65535},
	} {
		port, err := parsePort(LifecyclePortEnvVar, tc.raw, DefaultPort)
		require.NoError(t, err, "raw=%q", tc.raw)
		assert.Equal(t, tc.want, port)
	}
}

func TestParsePort_Lifecycle_RejectsInvalid(t *testing.T) {
	for _, raw := range []string{"abc", "0", "65536", "-1"} {
		_, err := parsePort(LifecyclePortEnvVar, raw, DefaultPort)
		assert.Error(t, err, "raw=%q should fail", raw)
		assert.ErrorContains(t, err, LifecyclePortEnvVar)
	}
}

// --- parseDurationMs ---

func TestParseDurationMs_UnsetReturnsDefault(t *testing.T) {
	got, err := parseDurationMs(ForwardTimeoutMsEnvVar, "", 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, got)
}

func TestParseDurationMs_ValidMs(t *testing.T) {
	got, err := parseDurationMs(ForwardTimeoutMsEnvVar, "5000", 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, got)
}

func TestParseDurationMs_RejectsNonNumeric(t *testing.T) {
	_, err := parseDurationMs(ForwardTimeoutMsEnvVar, "abc", 30*time.Second)
	require.Error(t, err)
	assert.ErrorContains(t, err, ForwardTimeoutMsEnvVar)
}

func TestParseDurationMs_RejectsZeroAndNegative(t *testing.T) {
	for _, raw := range []string{"0", "-1", "-1000"} {
		_, err := parseDurationMs(ForwardTimeoutMsEnvVar, raw, 30*time.Second)
		assert.Error(t, err, "raw=%q should fail", raw)
	}
}

func TestParseDurationMs_AcceptsMaxRepresentableMs(t *testing.T) {
	maxMs := int64(math.MaxInt64 / time.Millisecond)
	got, err := parseDurationMs(ForwardTimeoutMsEnvVar, strconv.FormatInt(maxMs, 10), 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(maxMs)*time.Millisecond, got)
}

func TestParseDurationMs_RejectsOverflow(t *testing.T) {
	maxMs := int64(math.MaxInt64 / time.Millisecond)
	_, err := parseDurationMs(ForwardTimeoutMsEnvVar, strconv.FormatInt(maxMs+1, 10), 30*time.Second)
	require.Error(t, err)
	assert.ErrorContains(t, err, ForwardTimeoutMsEnvVar)
}
