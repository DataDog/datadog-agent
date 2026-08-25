// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// testConfig returns a minimal *config.Config for unit tests.
func testConfig() *config.Config {
	return &config.Config{
		DatadogSite: "datadoghq.com",
		OrgId:       1,
		APIKey:      "test-api-key",
	}
}

// noopCaptureTrigger is a captureTrigger stand-in for unit tests so that
// Run() can be exercised end-to-end without a real system-probe socket.
type noopCaptureTrigger struct{}

func (n *noopCaptureTrigger) Capture(_ context.Context, _ RunCaptureInputs) (int, int64, time.Duration, string, error) {
	return 0, 0, 0, "", nil
}

// newTestHandler builds a RunCaptureHandler wired to noopCaptureTrigger,
// bypassing the platform-specific captureTrigger (which requires a live
// system-probe socket on unix).
func newTestHandler() *RunCaptureHandler {
	handler := NewRunCaptureHandler(testConfig())
	handler.capture = &noopCaptureTrigger{}
	return handler
}

// newTask builds a minimal *types.Task with the given inputs map.
func newTask(inputs map[string]interface{}) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: inputs,
	}
	return task
}

// validInputs returns a set of inputs that should pass all validation.
func validInputs() map[string]interface{} {
	return map[string]interface{}{
		"bpfFilter":    "tcp port 443",
		"durationSecs": 10,
	}
}

func TestRunCaptureValidation_MissingBPFFilter(t *testing.T) {
	handler := newTestHandler()
	task := newTask(map[string]interface{}{
		"durationSecs": 10,
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bpfFilter")
}

func TestRunCaptureValidation_DurationTooLow(t *testing.T) {
	handler := newTestHandler()
	task := newTask(map[string]interface{}{
		"bpfFilter":    "tcp port 80",
		"durationSecs": 0,
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "durationSecs")
}

func TestRunCaptureValidation_DurationTooHigh(t *testing.T) {
	handler := newTestHandler()
	task := newTask(map[string]interface{}{
		"bpfFilter":    "tcp port 80",
		"durationSecs": 121,
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "durationSecs")
}

func TestRunCaptureValidation_ValidInputs(t *testing.T) {
	handler := newTestHandler()
	task := newTask(validInputs())

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result, ok := output.(*RunCaptureResult)
	require.True(t, ok, "expected output to be *RunCaptureResult")
	assert.NotEmpty(t, result.CaptureID, "CaptureID should be a non-empty UUID")
}

func TestRunCaptureDefaults(t *testing.T) {
	// Patch handler to expose the parsed inputs so we can inspect defaults.
	// Since Run() applies defaults in-place before the stub return path,
	// we verify the effect indirectly: the only observable difference today
	// is that the call succeeds and returns a valid result.  We also confirm
	// the constants match the expected defaults so callers rely on them.
	assert.Equal(t, defaultSnapLen, 256)
	assert.Equal(t, defaultMaxPackets, 50000)

	handler := newTestHandler()

	// SnapLen=0 and MaxPackets=0 are omitted; handler must apply defaults.
	task := newTask(map[string]interface{}{
		"bpfFilter":    "udp port 53",
		"durationSecs": 5,
		// snapLen and maxPackets intentionally absent (zero-value)
	})

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result, ok := output.(*RunCaptureResult)
	require.True(t, ok)
	// The stub result is returned after defaults are applied; the call
	// succeeding with SnapLen/MaxPackets omitted proves the defaults path
	// does not panic or error out.
	assert.NotEmpty(t, result.CaptureID)
}

func TestGetAction_RunCapture(t *testing.T) {
	bundle := NewPcap(testConfig())
	action := bundle.GetAction("runCapture")
	assert.NotNil(t, action, "GetAction('runCapture') should return a non-nil Action")
}

func TestGetAction_Unknown(t *testing.T) {
	bundle := NewPcap(testConfig())
	action := bundle.GetAction("nonexistent")
	assert.Nil(t, action, "GetAction('nonexistent') should return nil")
}
