// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	testopms "github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms/testing"
)

// TestHealthCheckLoop_HonorsRetryAfterMs verifies that the health check loop
// uses the X-Retry-After-Ms value from the server response as the next
// interval, rather than the configured HealthCheckInterval.
func TestHealthCheckLoop_HonorsRetryAfterMs(t *testing.T) {
	const (
		defaultIntervalMs = 5   // very short so the first tick fires quickly
		retryAfterMs      = 100 // server instructs a longer wait
	)

	callTimes := make(chan time.Time, 10)

	runner := &CommonRunner{
		opmsClient: &testopms.FakeOpmsClient{
			HealthCheckFn: func(_ context.Context) (*opms.HealthCheckData, error) {
				callTimes <- time.Now()
				return &opms.HealthCheckData{RetryAfter: retryAfterMs * time.Millisecond}, nil
			},
		},
		config: &config.Config{HealthCheckInterval: defaultIntervalMs},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go runner.healthCheckLoop(ctx)

	// Wait for two consecutive calls.
	t1 := <-callTimes
	t2 := <-callTimes
	cancel()

	gap := t2.Sub(t1)

	// The gap should reflect the server-requested RetryAfterMs (100 ms),
	// not the 5 ms configured interval.
	assert.GreaterOrEqual(t, gap, time.Duration(retryAfterMs/2)*time.Millisecond,
		"gap should be close to RetryAfterMs (%d ms), not the default interval (%d ms)", retryAfterMs, defaultIntervalMs)
	assert.Less(t, gap, 500*time.Millisecond,
		"gap should not be excessively long")
}

// TestHealthCheckLoop_DefaultIntervalWhenRetryAfterIsZero verifies that the
// loop falls back to HealthCheckInterval when the server does not send a
// retry-after hint (or sends 0).
func TestHealthCheckLoop_DefaultIntervalWhenRetryAfterIsZero(t *testing.T) {
	const defaultIntervalMs = 30

	callTimes := make(chan time.Time, 10)

	runner := &CommonRunner{
		opmsClient: &testopms.FakeOpmsClient{
			HealthCheckFn: func(_ context.Context) (*opms.HealthCheckData, error) {
				callTimes <- time.Now()
				return &opms.HealthCheckData{RetryAfter: 0}, nil
			},
		},
		config: &config.Config{HealthCheckInterval: defaultIntervalMs},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go runner.healthCheckLoop(ctx)

	t1 := <-callTimes
	t2 := <-callTimes
	cancel()

	gap := t2.Sub(t1)

	// With no retry-after hint the gap should be close to the default interval.
	assert.GreaterOrEqual(t, gap, time.Duration(defaultIntervalMs/2)*time.Millisecond,
		"gap should reflect the default HealthCheckInterval")
	assert.Less(t, gap, 500*time.Millisecond,
		"gap should not be excessively long")
}
