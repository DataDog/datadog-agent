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

// recordingCaptureTrigger captures the inputs Run() resolved, so tests can
// assert on defaulting rather than only on the returned result.
type recordingCaptureTrigger struct {
	got RunCaptureInputs
}

func (r *recordingCaptureTrigger) Capture(_ context.Context, in RunCaptureInputs) (int, int64, time.Duration, string, error) {
	r.got = in
	return 0, 0, 0, "", nil
}

func newRecordingHandler() (*RunCaptureHandler, *recordingCaptureTrigger) {
	handler := NewRunCaptureHandler(testConfig())
	rec := &recordingCaptureTrigger{}
	handler.capture = rec
	return handler, rec
}

// An omitted bpfFilter means "capture everything", not an error. The capture UI
// does not require the user to supply a filter, so the Capture API dispatches
// without one; compileBPFFilter treats "" as match-all.
func TestRunCapture_OmittedBPFFilterIsAccepted(t *testing.T) {
	handler, rec := newRecordingHandler()
	task := newTask(map[string]interface{}{
		"captureId":    "cap-1",
		"durationSecs": 10,
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)
	assert.Empty(t, rec.got.BPFFilter)
}

// The caller's captureId must reach the capture untouched — it is the only key
// that correlates the dispatch with the uploaded pcap once the action result
// has expired.
func TestRunCapture_CaptureIDIsHonoured(t *testing.T) {
	handler, _ := newRecordingHandler()
	task := newTask(map[string]interface{}{
		"captureId":    "cap-abc-123",
		"bpfFilter":    "icmp",
		"durationSecs": 5,
	})

	res, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)
	require.IsType(t, &RunCaptureResult{}, res)
	assert.Equal(t, "cap-abc-123", res.(*RunCaptureResult).CaptureID)
}

func TestRunCapture_CaptureIDGeneratedWhenAbsent(t *testing.T) {
	handler, _ := newRecordingHandler()
	task := newTask(map[string]interface{}{
		"bpfFilter":    "icmp",
		"durationSecs": 5,
	})

	res, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, res.(*RunCaptureResult).CaptureID)
}

// Neither cap may be left unbounded. Omitted, zero and negative all resolve to
// the defaults — negative especially, since these are converted to uint64
// downstream where a negative would wrap to an effectively infinite limit.
func TestRunCapture_CapsAlwaysBounded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		inputs map[string]interface{}
	}{
		{"omitted", map[string]interface{}{"durationSecs": 5}},
		{"zero", map[string]interface{}{"durationSecs": 5, "maxPackets": 0, "maxBytes": 0}},
		{"negative", map[string]interface{}{"durationSecs": 5, "maxPackets": -1, "maxBytes": -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, rec := newRecordingHandler()
			tc.inputs["captureId"] = "cap-1"

			_, err := handler.Run(context.Background(), newTask(tc.inputs), nil)
			require.NoError(t, err)
			assert.Equal(t, defaultMaxPackets, rec.got.MaxPackets)
			assert.Equal(t, int64(defaultMaxBytes), rec.got.MaxBytes)
		})
	}
}

func TestRunCapture_ExplicitCapsArePreserved(t *testing.T) {
	handler, rec := newRecordingHandler()
	task := newTask(map[string]interface{}{
		"captureId":    "cap-1",
		"durationSecs": 5,
		"maxPackets":   1234,
		"maxBytes":     567890,
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)
	assert.Equal(t, 1234, rec.got.MaxPackets)
	assert.Equal(t, int64(567890), rec.got.MaxBytes)
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

// TestRunCaptureDefaults asserts the values Run() actually hands to the
// capturer when the optional bounds are omitted.
//
// This previously checked the constants against hardcoded literals and then
// only that the call did not error, because there was no way to observe the
// resolved inputs. recordingCaptureTrigger removes that limitation, so assert
// on the real thing — duplicating the literals here just meant every change to
// a default had to be made in two places.
func TestRunCaptureDefaults(t *testing.T) {
	handler, rec := newRecordingHandler()

	task := newTask(map[string]interface{}{
		"captureId":    "cap-defaults",
		"bpfFilter":    "udp port 53",
		"durationSecs": 5,
		// snapLen, maxPackets and maxBytes intentionally absent
	})

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	assert.Equal(t, defaultSnapLen, rec.got.SnapLen)
	assert.Equal(t, defaultMaxPackets, rec.got.MaxPackets)
	assert.Equal(t, int64(defaultMaxBytes), rec.got.MaxBytes)

	result, ok := output.(*RunCaptureResult)
	require.True(t, ok)
	assert.Equal(t, "cap-defaults", result.CaptureID)
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
