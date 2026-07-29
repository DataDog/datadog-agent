// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && pcap && cgo && integration && dynamic

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunCapture_BasicCapture verifies that a capture with valid inputs returns
// a RunCaptureResult with non-negative packet count, at least 24 bytes (PCAP
// global header), and a positive reported duration.
func TestRunCapture_BasicCapture(t *testing.T) {
	handler := NewRunCaptureHandler(nil)
	task := newTask(map[string]interface{}{
		"bpfFilter":    "tcp",
		"durationSecs": 5,
		"snapLen":      256,
	})

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result, ok := output.(*RunCaptureResult)
	require.True(t, ok, "expected output to be *RunCaptureResult")

	assert.GreaterOrEqual(t, result.PacketCount, 0, "PacketCount should be non-negative")
	assert.GreaterOrEqual(t, result.FileSizeBytes, int64(24), "FileSizeBytes should be at least 24 (PCAP global header)")
	assert.Greater(t, result.DurationSecs, 0, "DurationSecs should be positive")
	assert.NotEmpty(t, result.CaptureID, "CaptureID should be a non-empty UUID")
}

// TestRunCapture_ShortDuration verifies that a 1-second capture completes in
// roughly 1–2 seconds and does not hang.
func TestRunCapture_ShortDuration(t *testing.T) {
	handler := NewRunCaptureHandler(nil)
	task := newTask(map[string]interface{}{
		"bpfFilter":    "tcp",
		"durationSecs": 1,
	})

	start := time.Now()
	output, err := handler.Run(context.Background(), task, nil)
	elapsed := time.Since(start)

	require.NoError(t, err)

	result, ok := output.(*RunCaptureResult)
	require.True(t, ok, "expected output to be *RunCaptureResult")
	assert.GreaterOrEqual(t, result.PacketCount, 0)

	// Allow a generous 3-second window to account for setup overhead while
	// still ensuring the capture doesn't hang indefinitely.
	assert.Less(t, elapsed, 3*time.Second, "1-second capture should complete within 3 seconds, took %s", elapsed)
}

// TestRunCapture_ContextCancellation verifies that cancelling the context
// causes Run() to return promptly rather than blocking for the full duration.
func TestRunCapture_ContextCancellation(t *testing.T) {
	handler := NewRunCaptureHandler(nil)
	task := newTask(map[string]interface{}{
		"bpfFilter":    "tcp",
		"durationSecs": 30, // long enough that it won't finish on its own
	})

	ctx, cancel := context.WithCancel(context.Background())

	type result struct {
		output interface{}
		err    error
	}
	done := make(chan result, 1)

	go func() {
		out, err := handler.Run(ctx, task, nil)
		done <- result{out, err}
	}()

	// Cancel after 500 ms — well before the 30-second duration.
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case res := <-done:
		// Run() returned — it may succeed with partial results or return an
		// error; either is acceptable as long as it didn't hang.
		t.Logf("Run() returned after context cancellation: err=%v", res.err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within 2 seconds after context was cancelled")
	}
}

// TestRunCapture_InvalidInterface verifies that specifying a non-existent
// network interface causes Run() to return an error mentioning "not found".
func TestRunCapture_InvalidInterface(t *testing.T) {
	handler := NewRunCaptureHandler(nil)
	task := newTask(map[string]interface{}{
		"bpfFilter":    "tcp",
		"durationSecs": 5,
		"interface":    "nonexistent_iface_xyz",
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found", "error should mention 'not found' for unknown interface")
}
