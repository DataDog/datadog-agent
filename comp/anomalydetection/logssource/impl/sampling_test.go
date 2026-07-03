// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logssourceimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logsfilter"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/stretchr/testify/assert"
)

// newMsg returns a minimal message with the given status and optional extra tags.
// A non-nil LogsConfig is required to avoid nil-pointer dereference in Origin.Tags().
func newMsg(status string, tags ...string) *message.Message {
	src := sources.NewLogSource("test", &logsconfig.LogsConfig{})
	origin := message.NewOrigin(src)
	origin.SetTags(tags)
	return message.NewMessage([]byte("test"), origin, status, 0)
}

// --- logsfilter.RateWindow ---

func TestRateWindowUnlimited(t *testing.T) {
	var w logsfilter.RateWindow
	for i := 0; i < 1000; i++ {
		assert.True(t, w.Allow(-1), "unlimited (-1) should always allow")
	}
}

func TestRateWindowDropAll(t *testing.T) {
	var w logsfilter.RateWindow
	for i := 0; i < 10; i++ {
		assert.False(t, w.Allow(0), "rate=0 should drop everything")
	}
}

func TestRateWindowCapEnforced(t *testing.T) {
	var w logsfilter.RateWindow
	// 5/s over 10s window = 50 messages per window
	allowed := 0
	for i := 0; i < 100; i++ {
		if w.Allow(5.0) {
			allowed++
		}
	}
	assert.Equal(t, 50, allowed, "exactly 50 messages should be allowed in a 10s window at 5/s")
}

func TestRateWindowExhausted(t *testing.T) {
	var w logsfilter.RateWindow
	// Allow 1/s (10 per window), exhaust the window, verify the 11th is blocked.
	for i := 0; i < 10; i++ {
		assert.True(t, w.Allow(1.0))
	}
	assert.False(t, w.Allow(1.0), "window should be full after 10 messages")
	// Window-reset behaviour after expiry is covered by logsfilter_test.go.
}

func TestRateWindowSubTenthPerSecond(t *testing.T) {
	var w logsfilter.RateWindow
	// 0.05/s → maxPerWindow = 0.5 → only 1 message allowed per window
	assert.True(t, w.Allow(0.05), "first message should pass")
	assert.False(t, w.Allow(0.05), "second message should be dropped (floor: 1/window)")
}

// --- logSampler / ShouldForward ---

func TestShouldForwardTraceTreatedLikeDebug(t *testing.T) {
	// With no min_severity filtering, trace passes just like debug.
	s := newLogSampler(
		sourceRateLimits{maxRateHigh: -1, maxRateMedium: -1, maxRateLow: -1},
		sourceRateLimits{maxRateHigh: -1, maxRateMedium: -1, maxRateLow: -1},
		nil,
	)
	assert.True(t, s.ShouldForward(newMsg("trace")), "trace must pass when no min_severity gate")
	assert.True(t, s.ShouldForward(newMsg("debug")), "debug must pass when no min_severity gate")
}

func TestShouldForwardMinSeverityAllLevels(t *testing.T) {
	cases := []struct {
		minSeverity string
		pass        []string
		drop        []string
	}{
		{
			minSeverity: "trace",
			pass:        []string{"trace", "debug", "info", "warn", "error", "critical"},
			drop:        []string{},
		},
		{
			minSeverity: "debug",
			pass:        []string{"debug", "info", "warn", "error"},
			drop:        []string{"trace"},
		},
		{
			minSeverity: "warn",
			pass:        []string{"warn", "error", "critical"},
			drop:        []string{"trace", "debug", "info"},
		},
		{
			minSeverity: "error",
			pass:        []string{"error", "critical", "fatal"},
			drop:        []string{"trace", "debug", "info", "warn"},
		},
	}
	for _, tc := range cases {
		limits := sourceRateLimits{minSeverity: tc.minSeverity, maxRateHigh: -1, maxRateMedium: -1, maxRateLow: -1}
		s := newLogSampler(limits, limits, nil)
		for _, status := range tc.pass {
			assert.True(t, s.ShouldForward(newMsg(status)), "min_severity=%q: %q should pass", tc.minSeverity, status)
		}
		for _, status := range tc.drop {
			assert.False(t, s.ShouldForward(newMsg(status)), "min_severity=%q: %q should be dropped", tc.minSeverity, status)
		}
	}
}

func TestShouldForwardUnlimitedPassesAll(t *testing.T) {
	s := newLogSampler(
		sourceRateLimits{maxRateHigh: -1, maxRateMedium: -1, maxRateLow: -1},
		sourceRateLimits{maxRateHigh: -1, maxRateMedium: -1, maxRateLow: -1},
		nil,
	)
	for _, status := range []string{"trace", "debug", "info", "warn", "error", "critical"} {
		assert.True(t, s.ShouldForward(newMsg(status)), "unlimited: %s should pass", status)
	}
}

func TestShouldForwardMinSeverityGate(t *testing.T) {
	limits := sourceRateLimits{
		minSeverity:   "warn",
		maxRateHigh:   -1,
		maxRateMedium: -1,
		maxRateLow:    -1,
	}
	s := newLogSampler(limits, limits, nil)

	assert.False(t, s.ShouldForward(newMsg("debug")))
	assert.False(t, s.ShouldForward(newMsg("info")))
	assert.True(t, s.ShouldForward(newMsg("warn")))
	assert.True(t, s.ShouldForward(newMsg("error")))
}

func TestShouldForwardKubeletVsContainer(t *testing.T) {
	kubeletLimits := sourceRateLimits{maxRateHigh: -1, maxRateMedium: -1, maxRateLow: 0} // drop debug for kubelet
	containerLimits := sourceRateLimits{maxRateHigh: -1, maxRateMedium: -1, maxRateLow: -1}
	s := newLogSampler(kubeletLimits, containerLimits, nil)

	kubeletMsg := newMsg("debug", "source:kubelet")
	containerMsg := newMsg("debug") // no kubelet tag

	assert.False(t, s.ShouldForward(kubeletMsg), "kubelet debug dropped by kubelet limits")
	assert.True(t, s.ShouldForward(containerMsg), "container debug passes container limits")
}

func TestShouldForwardRateLimitTriggerOnDropped(t *testing.T) {
	drops := map[string]int{}
	onDropped := func(source, priority string) {
		drops[source+"/"+priority]++
	}
	limits := sourceRateLimits{maxRateHigh: 1.0, maxRateMedium: 1.0, maxRateLow: 1.0} // 10/window each
	s := newLogSampler(limits, limits, onDropped)

	for i := 0; i < 20; i++ {
		s.ShouldForward(newMsg("warn"))                   // high tier
		s.ShouldForward(newMsg("info"))                   // medium tier
		s.ShouldForward(newMsg("debug"))                  // low tier
		s.ShouldForward(newMsg("warn", "source:kubelet")) // high tier + kubelet
	}

	assert.Equal(t, 10, drops["containers/high"], "10 high-tier drops expected")
	assert.Equal(t, 10, drops["containers/medium"], "10 medium-tier drops expected")
	assert.Equal(t, 10, drops["containers/low"], "10 low-tier drops expected")
	assert.Equal(t, 10, drops["kubelet/high"], "10 high-tier drops expected for kubelet")
}

func TestShouldForwardSeverityFilteredDropsDoNotFireOnDropped(t *testing.T) {
	var dropped []string
	onDropped := func(_ string, priority string) {
		dropped = append(dropped, priority)
	}
	// min_severity="warn": trace/debug/info are filtered before reaching the rate limiter
	limits := sourceRateLimits{minSeverity: "warn", maxRateHigh: -1, maxRateMedium: -1, maxRateLow: -1}
	s := newLogSampler(limits, limits, onDropped)

	for i := 0; i < 50; i++ {
		s.ShouldForward(newMsg("trace"))
		s.ShouldForward(newMsg("debug"))
		s.ShouldForward(newMsg("info"))
	}

	assert.Empty(t, dropped, "severity-filtered drops must never fire onDropped")
}

func TestShouldForwardDropAllRate(t *testing.T) {
	limits := sourceRateLimits{maxRateHigh: 0, maxRateMedium: 0, maxRateLow: 0}
	s := newLogSampler(limits, limits, nil)
	assert.False(t, s.ShouldForward(newMsg("warn")))
	assert.False(t, s.ShouldForward(newMsg("info")))
	assert.False(t, s.ShouldForward(newMsg("debug")))
}
