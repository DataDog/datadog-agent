// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	pcapuploader "github.com/DataDog/datadog-agent/comp/networkpcap/uploader/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// defaultSnapLen matches system-probe's ceiling; system-probe trims every
	// packet to its headers regardless.
	defaultSnapLen = 128
	// defaultMaxPackets keeps the capture openable in Wireshark, whose memory
	// use grows with packet count (roughly 0.5-1 KB per packet), not file size.
	defaultMaxPackets = 2_000_000
	// defaultMaxBytes backstops defaultMaxPackets: at ~76 bytes per header-only
	// record, 2M packets is ~152 MB. It binds first only when packets are
	// larger, e.g. with a raised snapLen.
	defaultMaxBytes = 160 * 1024 * 1024
	minDurationSecs = 1
	// maxDurationSecs leaves room for the upload within the runner's default
	// 60 s task timeout. The backend never requests more than 30 s.
	maxDurationSecs = 30
)

// captureTrigger triggers a packet capture on system-probe and returns its
// results. Platform-specific implementations live in run_capture_socket.go
// (unix, over system-probe's unix socket) and run_capture_stub.go (others).
type captureTrigger interface {
	Capture(ctx context.Context, inputs RunCaptureInputs) (captureOutcome, error)
}

// captureOutcome is what a finished capture produced. The counts come from
// system-probe, which alone knows about kernel drops and truncated headers.
type captureOutcome struct {
	PacketCount      int
	PacketsDropped   int
	HeadersTruncated int
	Errors           int
	FileSizeBytes    int64
	Duration         time.Duration
	// PcapPath is the local temp file holding the capture; the caller removes it.
	PcapPath string
	// Socket is the system-probe socket the capture was requested on
	// (socketCapture, socketMain or socketNone). It is set on failure too.
	Socket string
}

// RunCaptureHandler handles the runCapture action.
type RunCaptureHandler struct {
	uploader pcapuploader.Component
	hostname hostnameinterface.Component
	capture  captureTrigger
}

// NewRunCaptureHandler constructs a RunCaptureHandler. Uploaded captures are
// tagged with the host name from hostname.
func NewRunCaptureHandler(uploader pcapuploader.Component, hostname hostnameinterface.Component) *RunCaptureHandler {
	return &RunCaptureHandler{
		uploader: uploader,
		hostname: hostname,
		capture:  newCaptureTrigger(),
	}
}

// RunCaptureInputs holds the inputs for the runCapture action.
type RunCaptureInputs struct {
	// CaptureID identifies the uploaded pcap. Action results expire, so the
	// caller should supply it to find the capture afterwards; when empty a
	// random ID is generated.
	CaptureID    string `json:"captureId,omitempty"`
	BPFFilter    string `json:"bpfFilter"`
	DurationSecs int    `json:"durationSecs"`
	Interface    string `json:"interface,omitempty"`
	MaxPackets   int    `json:"maxPackets,omitempty"`
	MaxBytes     int64  `json:"maxBytes,omitempty"`
	SnapLen      int    `json:"snapLen,omitempty"`
}

// RunCaptureResult holds the outputs for the runCapture action.
type RunCaptureResult struct {
	CaptureID     string `json:"captureId"`
	PacketCount   int    `json:"packetCount"`
	FileSizeBytes int64  `json:"fileSizeBytes"`
	DurationSecs  int    `json:"durationActualSecs"`
	// PacketsDropped counts packets the kernel discarded because the capture
	// could not keep up. Non-zero means the capture is incomplete.
	PacketsDropped int `json:"packetsDropped"`
	// HeadersTruncated counts packets whose headers did not fit in snapLen.
	HeadersTruncated int `json:"headersTruncated"`
	// CaptureErrors counts packets that could not be read or written.
	CaptureErrors int `json:"captureErrors"`
}

// Run validates inputs, triggers the capture on system-probe and uploads the result.
func (h *RunCaptureHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	// Each run is counted once, with the reason set by the step that ended it.
	reason, socket := reasonInvalidInput, socketNone
	defer func() { captureOutcomes.Inc(reason, socket) }()

	inputs, err := types.ExtractInputs[RunCaptureInputs](task)
	if err != nil {
		return nil, err
	}

	// An empty bpfFilter is valid and captures everything.

	if inputs.DurationSecs < minDurationSecs || inputs.DurationSecs > maxDurationSecs {
		return nil, fmt.Errorf("durationSecs must be between %d and %d, got %d", minDurationSecs, maxDurationSecs, inputs.DurationSecs)
	}

	if inputs.SnapLen == 0 {
		inputs.SnapLen = defaultSnapLen
	}

	// A caller can lower the caps but not raise or disable them: non-positive
	// and larger values fall back to the defaults. A negative value would
	// otherwise wrap to a huge limit when converted to uint64.
	if inputs.MaxPackets <= 0 || inputs.MaxPackets > defaultMaxPackets {
		inputs.MaxPackets = defaultMaxPackets
	}
	if inputs.MaxBytes <= 0 || inputs.MaxBytes > defaultMaxBytes {
		inputs.MaxBytes = defaultMaxBytes
	}

	// A generated ID is only returned in the action result, which expires, so
	// the capture is hard to find later.
	captureID := inputs.CaptureID
	if captureID == "" {
		captureID = uuid.New().String()
		log.Warnf("pcap: no captureId supplied, generated %s; this capture is only retrievable by an unfiltered networkpcap track query once the action result expires", captureID)
	}

	out, err := h.capture.Capture(ctx, inputs)
	if out.Socket != "" {
		socket = out.Socket
	}
	if err != nil {
		reason = captureFailureReason(err)
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	if out.PcapPath != "" {
		defer os.Remove(out.PcapPath)

		if err := uploadCapture(ctx, h.uploader, h.hostname, out.PcapPath, captureID); err != nil {
			reason = uploadFailureReason(err)
			return nil, fmt.Errorf("sending capture %s to event platform: %w", captureID, err)
		}
	}

	reason = reasonSuccess

	return &RunCaptureResult{
		CaptureID:        captureID,
		PacketCount:      out.PacketCount,
		FileSizeBytes:    out.FileSizeBytes,
		DurationSecs:     int(out.Duration.Round(time.Second).Seconds()),
		PacketsDropped:   out.PacketsDropped,
		HeadersTruncated: out.HeadersTruncated,
		CaptureErrors:    out.Errors,
	}, nil
}
