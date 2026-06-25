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

func TestParseUserAppPort_UnsetReturnsZeroNoError(t *testing.T) {
	port, err := parseUserAppPort("")
	require.NoError(t, err)
	assert.Equal(t, 0, port) // 0 = "not set"
}

func TestParseUserAppPort_ValidPort(t *testing.T) {
	port, err := parseUserAppPort("8080")
	require.NoError(t, err)
	assert.Equal(t, 8080, port)
}

func TestParseUserAppPort_AcceptsLowerBound(t *testing.T) {
	port, err := parseUserAppPort("1")
	require.NoError(t, err)
	assert.Equal(t, 1, port)
}

func TestParseUserAppPort_AcceptsUpperBound(t *testing.T) {
	port, err := parseUserAppPort("65535")
	require.NoError(t, err)
	assert.Equal(t, 65535, port)
}

func TestParseUserAppPort_RejectsNonNumeric(t *testing.T) {
	_, err := parseUserAppPort("abc")
	require.Error(t, err)
	assert.ErrorContains(t, err, UserAppPortEnvVar)
	assert.ErrorContains(t, err, "abc")
}

func TestParseUserAppPort_RejectsZero(t *testing.T) {
	_, err := parseUserAppPort("0")
	require.Error(t, err)
	assert.ErrorContains(t, err, UserAppPortEnvVar)
	assert.ErrorContains(t, err, "0")
}

func TestParseUserAppPort_RejectsAboveRange(t *testing.T) {
	_, err := parseUserAppPort("65536")
	require.Error(t, err)
	assert.ErrorContains(t, err, UserAppPortEnvVar)
	assert.ErrorContains(t, err, "65536")
}

func TestParseUserAppPort_RejectsNegative(t *testing.T) {
	_, err := parseUserAppPort("-1")
	require.Error(t, err)
	assert.ErrorContains(t, err, UserAppPortEnvVar)
	assert.ErrorContains(t, err, "-1")
}

func TestParseUserAppPort_RejectsNineThousand(t *testing.T) {
	_, err := parseUserAppPort("9000")
	require.Error(t, err)
	assert.ErrorContains(t, err, UserAppPortEnvVar)
	assert.ErrorContains(t, err, "9000")
}
