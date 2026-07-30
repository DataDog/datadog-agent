// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logsfilter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateWindowResetAfterExpiry(t *testing.T) {
	var w RateWindow
	for i := 0; i < 10; i++ {
		assert.True(t, w.Allow(1.0))
	}
	assert.False(t, w.Allow(1.0), "window should be full after 10 messages at 1/s")

	w.mu.Lock()
	w.windowStart = time.Now().Add(-RateWindowDuration - time.Millisecond)
	w.mu.Unlock()

	assert.True(t, w.Allow(1.0), "should allow after window expires")
}

func TestPriorityBucketOrdering(t *testing.T) {
	// Values mirror pkg/util/log/types.LogLevel — verify the ordering is correct.
	assert.Less(t, int(TracePriority), int(DebugPriority))
	assert.Less(t, int(DebugPriority), int(InfoPriority))
	assert.Less(t, int(InfoPriority), int(WarnPriority))
	assert.Less(t, int(WarnPriority), int(ErrorPriority))
	assert.Less(t, int(ErrorPriority), int(CriticalPriority))
	assert.Less(t, int(CriticalPriority), int(OffPriority))
}

func TestBucketForStatus(t *testing.T) {
	cases := []struct {
		status string
		want   PriorityBucket
	}{
		{"trace", TracePriority},
		{"TRACE", TracePriority},
		{"debug", DebugPriority},
		{"info", InfoPriority},
		{"", InfoPriority}, // unrecognised → info
		{"unknown", InfoPriority},
		{"notice", WarnPriority},
		{"warn", WarnPriority},
		{"warning", WarnPriority},
		{"error", ErrorPriority},
		{"critical", CriticalPriority},
		{"fatal", CriticalPriority},
		{"alert", CriticalPriority},
		{"emergency", CriticalPriority},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, BucketForStatus(tc.status), "BucketForStatus(%q)", tc.status)
	}
}

func TestMinBucketForSeverity(t *testing.T) {
	// empty → below TracePriority (passes everything)
	assert.Less(t, int(MinBucketForSeverity("")), int(TracePriority))

	assert.Equal(t, TracePriority, MinBucketForSeverity("trace"))
	assert.Equal(t, DebugPriority, MinBucketForSeverity("debug"))
	assert.Equal(t, InfoPriority, MinBucketForSeverity("info"))
	assert.Equal(t, WarnPriority, MinBucketForSeverity("warn"))
	assert.Equal(t, WarnPriority, MinBucketForSeverity("warning"))
	assert.Equal(t, ErrorPriority, MinBucketForSeverity("error"))
	assert.Equal(t, CriticalPriority, MinBucketForSeverity("critical"))
	assert.Equal(t, CriticalPriority, MinBucketForSeverity("fatal"))
	assert.Equal(t, OffPriority, MinBucketForSeverity("off"))
	// unrecognised → WarnPriority (safe default)
	assert.Equal(t, WarnPriority, MinBucketForSeverity("garbage"))
}

func TestMinSeverityGatesCorrectly(t *testing.T) {
	cases := []struct {
		minSeverity string
		passStatus  []string
		dropStatus  []string
	}{
		{
			minSeverity: "trace",
			passStatus:  []string{"trace", "debug", "info", "warn", "error", "critical"},
			dropStatus:  []string{},
		},
		{
			minSeverity: "debug",
			passStatus:  []string{"debug", "info", "warn", "error", "critical"},
			dropStatus:  []string{"trace"},
		},
		{
			minSeverity: "info",
			passStatus:  []string{"info", "notice", "warn", "error", "critical"},
			dropStatus:  []string{"trace", "debug"},
		},
		{
			minSeverity: "warn",
			passStatus:  []string{"warn", "notice", "error", "critical"},
			dropStatus:  []string{"trace", "debug", "info"},
		},
		{
			minSeverity: "error",
			passStatus:  []string{"error", "critical", "fatal"},
			dropStatus:  []string{"trace", "debug", "info", "warn"},
		},
		{
			minSeverity: "critical",
			passStatus:  []string{"critical", "fatal", "alert", "emergency"},
			dropStatus:  []string{"trace", "debug", "info", "warn", "error"},
		},
		{
			minSeverity: "off",
			passStatus:  []string{},
			dropStatus:  []string{"trace", "debug", "info", "warn", "error", "critical"},
		},
	}
	for _, tc := range cases {
		minBucket := MinBucketForSeverity(tc.minSeverity)
		for _, s := range tc.passStatus {
			assert.GreaterOrEqual(t, int(BucketForStatus(s)), int(minBucket),
				"min_severity=%q: %q should pass", tc.minSeverity, s)
		}
		for _, s := range tc.dropStatus {
			assert.Less(t, int(BucketForStatus(s)), int(minBucket),
				"min_severity=%q: %q should be dropped", tc.minSeverity, s)
		}
	}
}

func TestRateTierForBucket(t *testing.T) {
	assert.Equal(t, "low", RateTierForBucket(TracePriority))
	assert.Equal(t, "low", RateTierForBucket(DebugPriority))
	assert.Equal(t, "medium", RateTierForBucket(InfoPriority))
	assert.Equal(t, "high", RateTierForBucket(WarnPriority))
	assert.Equal(t, "high", RateTierForBucket(ErrorPriority))
	assert.Equal(t, "high", RateTierForBucket(CriticalPriority))
}
