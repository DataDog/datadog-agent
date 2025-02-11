// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package module

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParams(t *testing.T) {
	def := defaultParams()

	values := url.Values{}
	params, err := parseParams(values)
	require.NoError(t, err)
	require.Equal(t, def, params)

	values.Set(heartbeatParam, "abc")
	_, err = parseParams(values)
	require.Error(t, err)

	values.Set(heartbeatParam, "0")
	params, err = parseParams(values)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), params.heartbeatTime)

	values.Set(heartbeatParam, "5")
	params, err = parseParams(values)
	require.NoError(t, err)
	require.Equal(t, 5*time.Second, params.heartbeatTime)

	params = defaultParams()
	params.heartbeatTime = 2 * time.Second
	params.updateQuery(values)
	params, err = parseParams(values)
	require.NoError(t, err)
	require.Equal(t, 2*time.Second, params.heartbeatTime)
}
