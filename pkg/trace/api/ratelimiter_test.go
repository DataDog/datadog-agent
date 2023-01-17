// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
)

func TestComputeRateLimiterRate(t *testing.T) {
	assert := assert.New(t)

	// [0] -> max: the value in the conf file
	// [1] -> current: the value reported by the watchdog
	// [2] -> rate: the current rate limiter sampling rate
	expected := map[struct {
		max     float64 // the value in the conf file
		current float64 // the value reported by the CPU watchdog
		rate    float64 // the current (pre)sampling rate
	}]struct {
		r float64
	}{
		// Various cases showing general usage
		{max: 0.1, current: 0.1, rate: 1}:     {r: 1},
		{max: 0.2, current: 0.1, rate: 1}:     {r: 1},
		{max: 0.1, current: 0.15, rate: 1}:    {r: 0.8333333333333334},
		{max: 0.1, current: 0.2, rate: 1}:     {r: 0.75},
		{max: 0.2, current: 1, rate: 1}:       {r: 0.6},
		{max: 0.1, current: 0.11, rate: 1}:    {r: 1},
		{max: 0.1, current: 0.09, rate: 1}:    {r: 1},
		{max: 0.1, current: 0.05, rate: 1}:    {r: 1},
		{max: 0.1, current: 0.11, rate: 0.5}:  {r: 0.5},
		{max: 0.1, current: 0.5, rate: 0.5}:   {r: 0.3},
		{max: 0.15, current: 0.05, rate: 0.5}: {r: 1},
		{max: 0.1, current: 0.05, rate: 0.1}:  {r: 0.15000000000000002},
		{max: 0.04, current: 0.05, rate: 1}:   {r: 0.8999999999999999},
		{max: 0.025, current: 0.05, rate: 1}:  {r: 0.75},
		{max: 0.01, current: 0.05, rate: 0.1}: {r: 0.060000000000000005},

		// Check it's back to 1 even if current sampling rate is close to 1
		{max: 0.01, current: 0.005, rate: 0.99}: {r: 1},

		// Anti-jittering thing (not doing anything if target is too close to current)
		{max: 5, current: 3, rate: 0.5}:   {r: 0.6666666666666667},
		{max: 5, current: 4, rate: 0.5}:   {r: 0.5625},
		{max: 5, current: 4.5, rate: 0.5}: {r: 0.5},
		{max: 5, current: 4.9, rate: 0.5}: {r: 0.5},
		{max: 5, current: 5, rate: 0.5}:   {r: 0.5},
		{max: 5, current: 5.1, rate: 0.5}: {r: 0.5},
		{max: 5, current: 5.5, rate: 0.5}: {r: 0.5},
		{max: 5, current: 6, rate: 0.5}:   {r: 0.45833333333333337},
		{max: 5, current: 7, rate: 0.5}:   {r: 0.4285714285714286},

		// What happens when sampling at very high rate, and how do we converge on this
		{max: 0.1, current: 1000000, rate: 1}:                  {r: 0.50000005},
		{max: 0.1, current: 500000, rate: 0.50000005}:          {r: 0.25000007500000504},
		{max: 0.1, current: 250000, rate: 0.25000007500000504}: {r: 0.1250000875000175},
		{max: 0.1, current: 125000, rate: 0.1250000875000175}:  {r: 0.06250009375004376},
		{max: 0.1, current: 65000, rate: 0.06250009375004376}:  {r: 0.05},
		{max: 0.1, current: 50000, rate: 0.05}:                 {r: 0.05},

		// not initialized yet, this is what happens at startup (no error, just default to 1)
		{max: 0.1, current: 0, rate: 0}: {r: 1},

		// invalid input, those should really *NEVER* happen, test is just defensive
		{max: 0, current: 0.1, rate: 0.1}:     {r: 1},
		{max: 0.1, current: -0.02, rate: 0.1}: {r: 1},
		{max: 0.1, current: 0.05, rate: -0.2}: {r: 1},
	}

	for k, v := range expected {
		r := computeRateLimitingRate(k.max, k.current, k.rate)
		assert.Equal(v.r, r, "bad pre sample rate for max=%f current=%f, rate=%f, got %v, expected %v", k.max, k.current, k.rate, r, v.r)
	}
}

func TestRateLimiterRace(t *testing.T) {
	var wg sync.WaitGroup

	const N = 1000
	ps := newRateLimiter()
	wg.Add(5)

	go func() {
		for i := 0; i < N; i++ {
			ps.SetTargetRate(0.5)
			time.Sleep(time.Microsecond)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < N; i++ {
			_ = ps.TargetRate()
			time.Sleep(time.Microsecond)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < N; i++ {
			_ = ps.RealRate()
			time.Sleep(time.Microsecond)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < N; i++ {
			_ = ps.Permits(42)
			time.Sleep(time.Microsecond)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < N; i++ {
			ps.decayScore()
			time.Sleep(time.Microsecond)
		}
		wg.Done()
	}()
	wg.Wait()
}

func TestRateLimiterActive(t *testing.T) {
	assert := assert.New(t)

	ps := newRateLimiter()
	ps.Permits(0)
	assert.False(ps.Active(), "no traces should be seen")
	ps.Permits(-1)
	assert.False(ps.Active(), "still nothing")
	ps.Permits(10)
	assert.True(ps.Active(), "we should now be active")
}

func TestRateLimiterPermits(t *testing.T) {
	assert := assert.New(t)

	ps := newRateLimiter()
	ps.SetTargetRate(0.2)
	assert.Equal(0.2, ps.RealRate(), "by default, RealRate returns wished rate")
	assert.True(ps.Permits(100), "always accept first payload")
	ps.decayScore()
	assert.False(ps.Permits(10), "refuse as this accepting this would make 100%")
	ps.decayScore()
	assert.Equal(0.898876404494382, ps.RealRate())
	assert.False(ps.Permits(290), "still refuse")
	ps.decayScore()
	assert.False(ps.Permits(99), "just below the limit")
	ps.decayScore()
	assert.True(ps.Permits(1), "should there be no decay, this one would be dropped, but with decay, the rate decreased as the recently dropped gain importance over the old initially accepted")
	ps.decayScore()
	assert.Equal(0.16365162139216005, ps.RealRate(), "well below 20%, again, decay speaks")
	assert.True(ps.Permits(1000000), "accepting payload with many traces")
	ps.decayScore()
	assert.Equal(0.9997119577953764, ps.RealRate(), "real rate is almost 1, as we accepted a hudge payload")
	assert.False(ps.Permits(100000), "rejecting, real rate is too high now")
	ps.decayScore()
	assert.Equal(0.8986487877795845, ps.RealRate(), "real rate should be now around 90%")
	assert.Equal(info.RateLimiterStats{
		TargetRate:          0.2,
		RecentPayloadsSeen:  4.492300911839488, // seen more than this... but decay in action
		RecentTracesSeen:    879284.5615616576,
		RecentTracesDropped: 89116.55620097058,
	}, ps.stats)
}
