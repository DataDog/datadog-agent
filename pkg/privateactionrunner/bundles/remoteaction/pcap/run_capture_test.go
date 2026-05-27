// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// stubEventPlatform is a no-op eventplatform.Component for unit tests.
type stubEventPlatform struct{}

func (s *stubEventPlatform) Get() (eventplatform.Forwarder, bool) { return nil, false }

// stubForwarder satisfies eventplatform.Forwarder; used when testing send paths.
type stubForwarder struct{}

func (s *stubForwarder) SendEventPlatformEvent(_ *message.Message, _ string) error        { return nil }
func (s *stubForwarder) SendEventPlatformEventBlocking(_ *message.Message, _ string) error { return nil }
func (s *stubForwarder) Purge() map[string][]*message.Message                              { return nil }

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
	handler := NewRunCaptureHandler(&stubEventPlatform{})
	task := newTask(map[string]interface{}{
		"durationSecs": 10,
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bpfFilter")
}

func TestRunCaptureValidation_DurationTooLow(t *testing.T) {
	handler := NewRunCaptureHandler(&stubEventPlatform{})
	task := newTask(map[string]interface{}{
		"bpfFilter":    "tcp port 80",
		"durationSecs": 0,
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "durationSecs")
}

func TestRunCaptureValidation_DurationTooHigh(t *testing.T) {
	handler := NewRunCaptureHandler(&stubEventPlatform{})
	task := newTask(map[string]interface{}{
		"bpfFilter":    "tcp port 80",
		"durationSecs": 121,
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "durationSecs")
}

func TestRunCaptureValidation_ValidInputs(t *testing.T) {
	handler := NewRunCaptureHandler(&stubEventPlatform{})
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

	handler := NewRunCaptureHandler(&stubEventPlatform{})

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
	bundle := NewPcap(&stubEventPlatform{})
	action := bundle.GetAction("runCapture")
	assert.NotNil(t, action, "GetAction('runCapture') should return a non-nil Action")
}

func TestGetAction_Unknown(t *testing.T) {
	bundle := NewPcap(&stubEventPlatform{})
	action := bundle.GetAction("nonexistent")
	assert.Nil(t, action, "GetAction('nonexistent') should return nil")
}
