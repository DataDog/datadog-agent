// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
	"unsafe"

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

func TestTruncateTelemetryTag(t *testing.T) {
	assert.Equal(t, "echo hi", truncateTelemetryTag("echo hi", 16))
	assert.Equal(t, "echo hi", truncateTelemetryTag("echo hi", 7))
	assert.Equal(t, "echo"+telemetryTruncatedMark, truncateTelemetryTag("echo hi", 4))
	assert.Equal(t, telemetryTruncatedMark, truncateTelemetryTag("echo hi", 0))

	// "é" is two bytes; a limit landing inside it backs off to the rune start.
	got := truncateTelemetryTag("aé", 2)
	assert.Equal(t, "a"+telemetryTruncatedMark, got)
	assert.True(t, utf8.ValidString(got))
}

func TestTruncateTelemetryTagDoesNotShareMemory(t *testing.T) {
	short := strings.Repeat("x", 8)
	long := strings.Repeat("x", 1<<16)
	for _, text := range []string{short, long} {
		got := truncateTelemetryTag(text, 1024)
		start := uintptr(unsafe.Pointer(unsafe.StringData(text)))
		gotStart := uintptr(unsafe.Pointer(unsafe.StringData(got)))
		assert.False(t, gotStart >= start && gotStart < start+uintptr(len(text)),
			"the tag must not keep the original string's memory reachable")
	}
}

func TestCommandFlagsTag(t *testing.T) {
	assert.Equal(t, "", commandFlagsTag(nil))
	assert.Equal(t, ",-n,--file,", commandFlagsTag([]string{"-n", "--file"}))
	assert.Equal(t, "", commandFlagsTag([]string{"--" + strings.Repeat("a", maxCommandFlagsTagBytes)}))

	many := make([]string, 1000)
	for i := range many {
		many[i] = "--flag"
	}
	got := commandFlagsTag(many)
	assert.LessOrEqual(t, len(got), maxCommandFlagsTagBytes)
	assert.Equal(t, strings.Repeat(",--flag", strings.Count(got, "--flag"))+",", got)
}
