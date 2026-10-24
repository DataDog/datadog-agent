// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetrySpanLimitIsConcurrentSafe(t *testing.T) {
	previous := telemetrySpansStarted.Swap(maxTelemetrySpans - 1)
	t.Cleanup(func() { telemetrySpansStarted.Store(previous) })

	ctx := context.Background()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			span, _ := startTelemetrySpan(ctx, "test")
			span.Finish(nil)
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, uint64(maxTelemetrySpans), telemetrySpansStarted.Load())
}

func TestRunContinuesAfterTelemetrySpanLimit(t *testing.T) {
	previous := telemetrySpansStarted.Swap(maxTelemetrySpans)
	t.Cleanup(func() { telemetrySpansStarted.Store(previous) })

	runner, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { runner.Close() })

	require.NoError(t, runner.Run(context.Background(), parseScript(t, "true")))
	assert.Equal(t, uint64(maxTelemetrySpans), telemetrySpansStarted.Load())
}
