// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

const (
	defaultSnapLen    = 256
	defaultMaxPackets = 50000
	minDurationSecs   = 1
	maxDurationSecs   = 120
)

// RunCaptureHandler handles the runCapture action.
type RunCaptureHandler struct {
	eventPlatform eventplatform.Component
}

// NewRunCaptureHandler constructs a RunCaptureHandler.
func NewRunCaptureHandler(eventPlatform eventplatform.Component) *RunCaptureHandler {
	return &RunCaptureHandler{
		eventPlatform: eventPlatform,
	}
}

// RunCaptureInputs holds the inputs for the runCapture action.
type RunCaptureInputs struct {
	BPFFilter    string `json:"bpfFilter"`
	DurationSecs int    `json:"durationSecs"`
	Interface    string `json:"interface,omitempty"`
	MaxPackets   int    `json:"maxPackets,omitempty"`
	SnapLen      int    `json:"snapLen,omitempty"`
	SendToLogs   bool   `json:"sendToLogs,omitempty"`
}

// RunCaptureResult holds the outputs for the runCapture action.
type RunCaptureResult struct {
	CaptureID     string `json:"captureId"`
	PacketCount   int    `json:"packetCount"`
	FileSizeBytes int64  `json:"fileSizeBytes"`
	DurationSecs  int    `json:"durationActualSecs"`
	AttachmentKey string `json:"attachmentKey,omitempty"`
}

// Run validates inputs and performs a packet capture via the platform-specific doCapture helper.
func (h *RunCaptureHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RunCaptureInputs](task)
	if err != nil {
		return nil, err
	}

	if inputs.BPFFilter == "" {
		return nil, errors.New("bpfFilter is required")
	}

	if inputs.DurationSecs < minDurationSecs || inputs.DurationSecs > maxDurationSecs {
		return nil, fmt.Errorf("durationSecs must be between %d and %d, got %d", minDurationSecs, maxDurationSecs, inputs.DurationSecs)
	}

	if inputs.SnapLen == 0 {
		inputs.SnapLen = defaultSnapLen
	}

	if inputs.MaxPackets == 0 {
		inputs.MaxPackets = defaultMaxPackets
	}

	captureID := uuid.New().String()

	packetCount, fileSizeBytes, actualDuration, err := doCapture(ctx, inputs)
	if err != nil {
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	actualSecs := int(actualDuration.Round(time.Second).Seconds())

	return &RunCaptureResult{
		CaptureID:     captureID,
		PacketCount:   packetCount,
		FileSizeBytes: fileSizeBytes,
		DurationSecs:  actualSecs,
		AttachmentKey: "",
	}, nil
}
