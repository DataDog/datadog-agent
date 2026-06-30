// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/monitor"
)

func TestEgressPolicyStartsForwardingAndAppliesSendDelay(t *testing.T) {
	start := time.Unix(100, 0)
	policy := NewEgressPolicy(EgressPolicyOptions{SendDelay: 10 * time.Second})

	require.Equal(t, EgressForwarding, policy.Mode())
	require.Equal(t, []TimeRange{{To: start}}, policy.RangesToForward(start.Add(10*time.Second)))
}

func TestEgressPolicyMarksForwardedRangesWithoutDuplicates(t *testing.T) {
	start := time.Unix(100, 0)
	policy := NewEgressPolicy(EgressPolicyOptions{SendDelay: time.Nanosecond})

	first := TimeRange{From: start, To: start.Add(10 * time.Second)}
	policy.MarkForwarded(first)

	ranges := policy.RangesToForward(start.Add(20 * time.Second))
	require.NotContains(t, ranges, first)
	for _, r := range ranges {
		require.False(t, rangesOverlap(r, first))
	}
}

func TestEgressPolicySuppressesAfterConsecutiveHealthyWindows(t *testing.T) {
	start := time.Unix(100, 0)
	policy := NewEgressPolicy(EgressPolicyOptions{
		SendDelay:                time.Nanosecond,
		HealthyWindowsToSuppress: 2,
		PostWindow:               5 * time.Second,
	})

	policy.OnDecision(monitor.Decision{State: monitor.Healthy, WindowFrom: start, WindowTo: start.Add(15 * time.Second)})
	require.Equal(t, EgressForwarding, policy.Mode())

	policy.OnDecision(monitor.Decision{State: monitor.Healthy, WindowFrom: start.Add(15 * time.Second), WindowTo: start.Add(30 * time.Second)})
	require.Equal(t, EgressSuppressed, policy.Mode())
	require.Equal(t, []TimeRange{{To: start.Add(35 * time.Second)}}, policy.ForwardingRanges())
}

func TestEgressPolicyBreachReopensSuppressedEgressWithPreWindow(t *testing.T) {
	start := time.Unix(100, 0)
	policy := suppressedPolicy(start)

	policy.OnDecision(monitor.Decision{
		State:      monitor.Breach,
		WindowFrom: start.Add(60 * time.Second),
		WindowTo:   start.Add(75 * time.Second),
	})

	require.Equal(t, EgressForwarding, policy.Mode())
	require.Equal(t, []TimeRange{
		{To: start.Add(30 * time.Second)},
		{From: start.Add(55 * time.Second)},
	}, policy.ForwardingRanges())
}

func TestEgressPolicyUnknownReopensSuppressedEgress(t *testing.T) {
	start := time.Unix(100, 0)
	policy := suppressedPolicy(start)

	policy.OnDecision(monitor.Decision{
		State:      monitor.Unknown,
		WindowFrom: start.Add(60 * time.Second),
		WindowTo:   start.Add(75 * time.Second),
	})

	require.Equal(t, EgressForwarding, policy.Mode())
	require.Equal(t, []TimeRange{
		{To: start.Add(30 * time.Second)},
		{From: start.Add(55 * time.Second)},
	}, policy.ForwardingRanges())
}

func TestEgressPolicyStaleMonitorReopensSuppressedEgress(t *testing.T) {
	start := time.Unix(100, 0)
	policy := suppressedPolicy(start)

	require.False(t, policy.MarkStaleIfNeeded(start.Add(40*time.Second)))
	require.Equal(t, EgressSuppressed, policy.Mode())

	require.True(t, policy.MarkStaleIfNeeded(start.Add(61*time.Second)))
	require.Equal(t, EgressForwarding, policy.Mode())
	require.Equal(t, []TimeRange{{}}, policy.ForwardingRanges())
}

func TestEgressPolicyMergesOverlappingForwardingRanges(t *testing.T) {
	start := time.Unix(100, 0)
	policy := suppressedPolicy(start)

	policy.OnDecision(monitor.Decision{
		State:      monitor.Breach,
		WindowFrom: start.Add(25 * time.Second),
		WindowTo:   start.Add(40 * time.Second),
	})

	require.Equal(t, []TimeRange{{}}, policy.ForwardingRanges())
}

func TestEgressPolicyHalfOpenBoundariesDoNotOverlapForwardedRanges(t *testing.T) {
	start := time.Unix(100, 0)
	policy := NewEgressPolicy(EgressPolicyOptions{SendDelay: time.Nanosecond})
	policy.MarkForwarded(TimeRange{From: start, To: start.Add(10 * time.Second)})

	ranges := policy.RangesToForward(start.Add(20 * time.Second))
	for _, r := range ranges {
		require.False(t, rangesOverlap(r, TimeRange{From: start, To: start.Add(10 * time.Second)}))
	}
}

func suppressedPolicy(start time.Time) *EgressPolicy {
	policy := NewEgressPolicy(EgressPolicyOptions{
		SendDelay:                time.Nanosecond,
		HealthyWindowsToSuppress: 1,
		PreWindow:                5 * time.Second,
		MonitorStaleTimeout:      30 * time.Second,
	})
	policy.OnDecision(monitor.Decision{State: monitor.Healthy, WindowFrom: start, WindowTo: start.Add(30 * time.Second)})
	return policy
}
