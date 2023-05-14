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

	st := NewSimpleThrottler(3, time.Minute, "message")

	// iterate multiple times on the same throttler to validate it is resetting properly
	for i := 0; i < 5; i++ {
		throttled, limit := st.shouldThrottle()
		require.False(throttled, "should not be throttled")
		require.False(limit, "should not be throttled")
		throttled, limit = st.shouldThrottle()
		require.False(throttled, "should not be throttled")
		require.False(limit, "should not be throttled")
		throttled, limit = st.shouldThrottle()
		require.True(throttled, "should be throttled")
		require.True(limit, "should have just been throttled")
		throttled, limit = st.shouldThrottle()
		require.True(throttled, "should be throttled")
		require.False(limit, "throttle should have happened in the past")
		throttled, limit = st.shouldThrottle()
		require.True(throttled, "should be throttled")
		require.False(limit, "throttle should have happened in the past")
		clk.Add(time.Second * 30)
		throttled, limit = st.shouldThrottle()
		require.True(throttled, "should be throttled")
		require.False(limit, "throttle should have happened in the past")
		clk.Add(time.Minute)
		// it should now be reset
	}

	for i := 0; i < 5; i++ {
		require.False(st.ShouldThrottle())
		require.False(st.ShouldThrottle())
		require.False(st.ShouldThrottle())
		require.True(st.ShouldThrottle())
		require.True(st.ShouldThrottle())
		clk.Add(2 * time.Minute)
		// it should now be reset
	}
}
