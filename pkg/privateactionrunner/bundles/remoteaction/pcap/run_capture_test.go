// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fileupload "github.com/DataDog/datadog-agent/comp/forwarder/fileupload/def"
	fileuploadmock "github.com/DataDog/datadog-agent/comp/forwarder/fileupload/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// noopCaptureTrigger is a captureTrigger stand-in for unit tests so that
// Run() can be exercised end-to-end without a real system-probe socket.
type noopCaptureTrigger struct{}

func (n *noopCaptureTrigger) Capture(_ context.Context, _ RunCaptureInputs) (captureOutcome, error) {
	return captureOutcome{}, nil
}

// newTestHandler builds a RunCaptureHandler wired to noopCaptureTrigger,
// bypassing the platform-specific captureTrigger (which requires a live
// system-probe socket on unix).
func newTestHandler() *RunCaptureHandler {
	handler := NewRunCaptureHandler(fileuploadmock.New())
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
// assert on defaulting rather than only on the returned result. It returns out.
type recordingCaptureTrigger struct {
	got RunCaptureInputs
	out captureOutcome
}

func (r *recordingCaptureTrigger) Capture(_ context.Context, in RunCaptureInputs) (captureOutcome, error) {
	r.got = in
	return r.out, nil
}

func newRecordingHandler() (*RunCaptureHandler, *recordingCaptureTrigger) {
	handler := NewRunCaptureHandler(fileuploadmock.New())
	rec := &recordingCaptureTrigger{}
	handler.capture = rec
	return handler, rec
}

// An omitted bpfFilter means "capture everything", not an error. Callers may
// dispatch without one.
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

// system-probe's drop, truncation and error counts must reach the action
// result: a non-zero drop count is the only sign that a capture is incomplete.
func TestRunCapture_ReportsCaptureCounts(t *testing.T) {
	handler, rec := newRecordingHandler()
	rec.out = captureOutcome{
		PacketCount:      100,
		PacketsDropped:   7,
		HeadersTruncated: 3,
		Errors:           1,
		FileSizeBytes:    4096,
		Duration:         5 * time.Second,
	}

	res, err := handler.Run(context.Background(), newTask(validInputs()), nil)
	require.NoError(t, err)
	assert.Equal(t, &RunCaptureResult{
		CaptureID:        res.(*RunCaptureResult).CaptureID,
		PacketCount:      100,
		FileSizeBytes:    4096,
		DurationSecs:     5,
		PacketsDropped:   7,
		HeadersTruncated: 3,
		CaptureErrors:    1,
	}, res)
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

// A caller can lower the caps but not raise them.
func TestRunCapture_LargerCapsAreLowered(t *testing.T) {
	handler, rec := newRecordingHandler()
	task := newTask(map[string]interface{}{
		"captureId":    "cap-1",
		"durationSecs": 5,
		"maxPackets":   1e12,
		"maxBytes":     1e12,
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)
	assert.Equal(t, defaultMaxPackets, rec.got.MaxPackets)
	assert.Equal(t, int64(defaultMaxBytes), rec.got.MaxBytes)
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
	bundle := NewPcap(fileuploadmock.New())
	action := bundle.GetAction("runCapture")
	assert.NotNil(t, action, "GetAction('runCapture') should return a non-nil Action")
}

func TestGetAction_Unknown(t *testing.T) {
	bundle := NewPcap(fileuploadmock.New())
	action := bundle.GetAction("nonexistent")
	assert.Nil(t, action, "GetAction('nonexistent') should return nil")
}

// writeCapture writes data to a temp file standing in for a finished capture.
func writeCapture(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.pcap")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

// A finished capture is uploaded to the networkpcap track as one pcap
// attachment, with the capture ID in the event, and then removed.
func TestRunCapture_UploadsCapture(t *testing.T) {
	uploader := fileuploadmock.New()
	handler := NewRunCaptureHandler(uploader)
	pcapPath := writeCapture(t, []byte("pcap bytes"))
	handler.capture = &recordingCaptureTrigger{out: captureOutcome{PcapPath: pcapPath}}

	_, err := handler.Run(context.Background(), newTask(map[string]interface{}{
		"captureId":    "cap-upload",
		"durationSecs": 5,
	}), nil)
	require.NoError(t, err)

	uploads := uploader.Uploads()
	require.Len(t, uploads, 1)
	upload := uploads[0]
	assert.Equal(t, fileupload.TrackNetworkPcap, upload.Track)

	var event networkPcapEvent
	require.NoError(t, json.Unmarshal(upload.Event, &event))
	assert.Equal(t, "networkpcap", event.DDSource)
	assert.Equal(t, "cap-upload", event.CaptureID)
	assert.NotZero(t, event.Timestamp)

	require.Len(t, upload.Attachments, 1)
	assert.Equal(t, "pcap", upload.Attachments[0].Name)
	assert.Equal(t, "capture.pcap", upload.Attachments[0].Filename)
	assert.Equal(t, []byte("pcap bytes"), upload.Attachments[0].Content)

	assert.NoFileExists(t, pcapPath, "the capture file must be removed after the upload")
}

// A failed upload fails the action: the capture cannot be re-taken, so the
// caller must know it was lost. The file is still removed.
func TestRunCapture_UploadFailureFailsTheAction(t *testing.T) {
	uploader := fileuploadmock.New()
	uploader.SetError(errors.New("intake unavailable"))
	handler := NewRunCaptureHandler(uploader)
	pcapPath := writeCapture(t, []byte("pcap bytes"))
	handler.capture = &recordingCaptureTrigger{out: captureOutcome{PcapPath: pcapPath}}

	_, err := handler.Run(context.Background(), newTask(validInputs()), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intake unavailable")
	assert.NoFileExists(t, pcapPath)
}

func TestRunCapture_NoUploaderFailsTheAction(t *testing.T) {
	handler := NewRunCaptureHandler(nil)
	handler.capture = &recordingCaptureTrigger{out: captureOutcome{PcapPath: writeCapture(t, []byte("pcap"))}}

	_, err := handler.Run(context.Background(), newTask(validInputs()), nil)
	require.Error(t, err)
}
