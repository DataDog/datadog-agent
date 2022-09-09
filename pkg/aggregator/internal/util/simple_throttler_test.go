// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
)

func TestSimpleThrottler(t *testing.T) {
	require := require.New(t)

	clk := clock.NewMock()
	timeNow = clk.Now

	st := SimpleThrottler{
		ExecLimit:     3,
		PauseDuration: time.Minute * 1,
	}

	// iterate multiple times on the same throttler to validate it is resetting properly
	for i := 0; i < 5; i++ {
		throttled, limit := st.ShouldThrottle()
		require.False(throttled, "should not be throttled")
		require.False(limit, "should not be throttled")
		throttled, limit = st.ShouldThrottle()
		require.False(throttled, "should not be throttled")
		require.False(limit, "should not be throttled")
		throttled, limit = st.ShouldThrottle()
		require.True(throttled, "should be throttled")
		require.True(limit, "should have just been throttled")
		throttled, limit = st.ShouldThrottle()
		require.True(throttled, "should be throttled")
		require.False(limit, "throttle should have happened in the past")
		throttled, limit = st.ShouldThrottle()
		require.True(throttled, "should be throttled")
		require.False(limit, "throttle should have happened in the past")
		clk.Add(time.Second * 30)
		throttled, limit = st.ShouldThrottle()
		require.True(throttled, "should be throttled")
		require.False(limit, "throttle should have happened in the past")
		clk.Add(time.Minute)
		// it should now be reset
	}
}
