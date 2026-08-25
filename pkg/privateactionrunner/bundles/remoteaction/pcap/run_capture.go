// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

const (
	defaultSnapLen    = 256
	defaultMaxPackets = 50000
	minDurationSecs   = 1
	maxDurationSecs   = 120
)

// captureTrigger triggers a packet capture on system-probe and returns its
// results. Platform-specific implementations live in run_capture_socket.go
// (unix, over system-probe's unix socket) and run_capture_stub.go (others).
type captureTrigger interface {
	Capture(ctx context.Context, inputs RunCaptureInputs) (packetCount int, fileSizeBytes int64, actualDuration time.Duration, pcapPath string, err error)
}

// RunCaptureHandler handles the runCapture action.
type RunCaptureHandler struct {
	uploader *networkPcapUploader
	capture  captureTrigger
}

// NewRunCaptureHandler constructs a RunCaptureHandler.
func NewRunCaptureHandler(cfg *config.Config) *RunCaptureHandler {
	return &RunCaptureHandler{
		uploader: newNetworkPcapUploader(cfg),
		capture:  newCaptureTrigger(),
	}
}

// RunCaptureInputs holds the inputs for the runCapture action.
type RunCaptureInputs struct {
	BPFFilter    string `json:"bpfFilter"`
	DurationSecs int    `json:"durationSecs"`
	Interface    string `json:"interface,omitempty"`
	MaxPackets   int    `json:"maxPackets,omitempty"`
	SnapLen      int    `json:"snapLen,omitempty"`
}

// RunCaptureResult holds the outputs for the runCapture action.
type RunCaptureResult struct {
	CaptureID     string `json:"captureId"`
	PacketCount   int    `json:"packetCount"`
	FileSizeBytes int64  `json:"fileSizeBytes"`
	DurationSecs  int    `json:"durationActualSecs"`
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

	packetCount, fileSizeBytes, actualDuration, pcapPath, err := h.capture.Capture(ctx, inputs)
	if err != nil {
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	actualSecs := int(actualDuration.Round(time.Second).Seconds())

	if pcapPath != "" {
		defer os.Remove(pcapPath)

		if err := h.sendCapture(ctx, pcapPath, captureID); err != nil {
			return nil, fmt.Errorf("sending capture %s to event platform: %w", captureID, err)
		}
	}

	return &RunCaptureResult{
		CaptureID:     captureID,
		PacketCount:   packetCount,
		FileSizeBytes: fileSizeBytes,
		DurationSecs:  actualSecs,
	}, nil
}

// sendCapture uploads the pcap file at pcapPath to the "networkpcap" EVP
// attachment track via its multipart intake endpoint.
func (h *RunCaptureHandler) sendCapture(ctx context.Context, pcapPath string, captureID string) error {
	pcapBytes, err := os.ReadFile(pcapPath)
	if err != nil {
		return fmt.Errorf("reading captured pcap file: %w", err)
	}

	if err := h.uploader.Upload(ctx, pcapBytes, captureID); err != nil {
		return fmt.Errorf("uploading pcap to event platform: %w", err)
	}

	return nil
}
