// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
)

func TestLatencyThrottledSenderLowLatencyOneWorker(t *testing.T) {
	pool := newLatencyThrottledSenderPoolWithOptions(10*time.Millisecond, 0.055, 1, 100*time.Millisecond, client.NewNoopDestinationMetadata())

	for i := 0; i < 100; i++ {
		pool.Run(func() {
			time.Sleep(10 * time.Millisecond)
		})
	}

	// Should converge on a virtual latency of 10ms since there is only 1 worker, so virtual latency == real latency.
	require.InDelta(t, 10*time.Millisecond, pool.virtualLatency, float64(3*time.Millisecond))
}

func TestLatencyThrottledSenderPoolLowLatencyManyWorkers(t *testing.T) {
	pool := newLatencyThrottledSenderPoolWithOptions(10*time.Millisecond, 0.055, 10, 100*time.Millisecond, client.NewNoopDestinationMetadata())

	for i := 0; i < 100; i++ {
		pool.Run(func() {
			time.Sleep(10 * time.Millisecond)
		})
	}

	// Should converge on a virtual latency of 10ms since since target latency is 100ms and real latency is 10ms. No need to scale up.
	require.InDelta(t, 10*time.Millisecond, pool.virtualLatency, float64(3*time.Millisecond))
}

func TestLatencyThrottledSenderPoolScalesUpWorkersForHighLatency(t *testing.T) {
	pool := newLatencyThrottledSenderPoolWithOptions(10*time.Millisecond, 0.055, 10, 2*time.Millisecond, client.NewNoopDestinationMetadata())

	for i := 0; i < 1000; i++ {
		pool.Run(func() {
			time.Sleep(10 * time.Millisecond)
		})
	}
	// Should converge on virtual latency of 2ms since 2ms is the target, and there are enough workers to scale up.
	require.InDelta(t, 2*time.Millisecond, pool.virtualLatency, float64(500*time.Microsecond))
}

func TestLatencyThrottledSenderPoolStarvedForWorkers(t *testing.T) {
	pool := newLatencyThrottledSenderPoolWithOptions(10*time.Millisecond, 0.055, 1, 2*time.Millisecond, client.NewNoopDestinationMetadata())

	for i := 0; i < 100; i++ {
		pool.Run(func() {
			time.Sleep(10 * time.Millisecond)
		})
	}
	// Should converge on virtual latency of 10ms because there are not enough workers to scale up
	require.InDelta(t, 10*time.Millisecond, pool.virtualLatency, float64(3*time.Millisecond))
}
