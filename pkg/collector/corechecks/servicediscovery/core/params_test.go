// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package core

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHeartbeatParams(t *testing.T) {
	def := DefaultParams()

	values := url.Values{}
	params, err := ParseParams(values)
	require.NoError(t, err)
	require.Equal(t, def, params)

	values.Set(heartbeatParam, "abc")
	_, err = ParseParams(values)
	require.Error(t, err)

	values.Set(heartbeatParam, "0")
	params, err = ParseParams(values)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), params.HeartbeatTime)

	values.Set(heartbeatParam, "5")
	params, err = ParseParams(values)
	require.NoError(t, err)
	require.Equal(t, 5*time.Second, params.HeartbeatTime)

	params = DefaultParams()
	params.HeartbeatTime = 2 * time.Second
	params.UpdateQuery(values)
	params, err = ParseParams(values)
	require.NoError(t, err)
	require.Equal(t, 2*time.Second, params.HeartbeatTime)
}

func TestPidsParams(t *testing.T) {
	def := DefaultParams()

	values := url.Values{}
	params, err := ParseParams(values)
	require.NoError(t, err)
	require.Equal(t, def, params)

	values.Set(pidsParam, "1,2,3")
	params, err = ParseParams(values)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, params.Pids)
}
