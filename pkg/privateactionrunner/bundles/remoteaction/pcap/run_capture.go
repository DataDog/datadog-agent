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

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultSnapLen = 256
	// defaultMaxPackets and defaultMaxBytes bound an otherwise unbounded capture.
	// The constraint they serve is not the pipeline — which ships 100 MB happily —
	// but the usability of the artefact: Wireshark's limit is packet count, not
	// file size, and it costs roughly 0.5-1 KB of RAM per packet for dissection
	// state. Header-only capture packs ~9x more packets into a megabyte than an
	// ordinary full-packet capture, so file-size intuitions from normal pcaps do
	// not transfer.
	//
	// 1M packets is ~1 GB of Wireshark RAM: it loads in 10-30s and stays usable.
	// 3.5M is minutes-to-load and re-dissects on every filter change; 9M swaps or
	// OOMs a 16 GB laptop. 1M is therefore the largest round number that keeps the
	// download openable, and is the floor of the 1-3M per-capture band in
	// ai-investigations/remote-pcap/notes.md.
	//
	// This still truncates the worst hosts: one busy host captured unfiltered
	// produces ~6.18M packets in 30s, so a full-duration window there needs a BPF
	// filter, not a higher cap. Above ~1M the artefact stops being openable in the
	// GUI at all, which is a worse failure than a short window.
	defaultMaxPackets = 1_000_000
	// defaultMaxBytes is the largest upload we are willing to ship. It backstops
	// defaultMaxPackets rather than binding first: at ~76 bytes per header-only
	// record, 1M packets is ~76 MB, comfortably under. It takes effect when
	// packets are larger than the header-only assumption (e.g. a raised snapLen),
	// which is exactly the case where a packet count is the wrong guardrail.
	//
	// Sizing note: the intake's real ceiling is a 30s edge timeout rather than a
	// content-length limit, so 100 MiB assumes the host can sustain ~28 Mbit/s to
	// the intake. A slow uplink fails on time, not on size.
	defaultMaxBytes = 100 * 1024 * 1024
	minDurationSecs = 1
	maxDurationSecs = 120
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
	// CaptureID correlates this capture with the pcap the Agent uploads to the
	// networkpcap EVP track. The caller mints it because Action Platform task
	// results expire after 4 hours while the captured bytes are retained far
	// longer, so the track — keyed on this value — is the only durable way to
	// find a capture again. Optional on the wire for backwards compatibility;
	// when empty the Agent falls back to a locally generated UUID, which is
	// only retrievable by an unfiltered track query.
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

	// An empty bpfFilter is valid and means "capture everything" — the UI does not
	// require the user to supply one, so the Capture API may dispatch without it.
	// compileBPFFilter treats "" as match-all rather than as an error.

	if inputs.DurationSecs < minDurationSecs || inputs.DurationSecs > maxDurationSecs {
		return nil, fmt.Errorf("durationSecs must be between %d and %d, got %d", minDurationSecs, maxDurationSecs, inputs.DurationSecs)
	}

	if inputs.SnapLen == 0 {
		inputs.SnapLen = defaultSnapLen
	}

	// Neither cap can be disabled. Any non-positive value falls back to the
	// default rather than meaning "unbounded", because with no BPF filter
	// unbounded is millions of packets on a busy host. Treating <= 0 rather than
	// == 0 as "unset" also closes a sharp edge: these are converted to uint64
	// before reaching the capturer, so a negative would wrap to an enormous
	// limit and disable the cap by accident.
	if inputs.MaxPackets <= 0 {
		inputs.MaxPackets = defaultMaxPackets
	}
	if inputs.MaxBytes <= 0 {
		inputs.MaxBytes = defaultMaxBytes
	}

	// Prefer the caller's ID. Minting our own is a fallback for callers that
	// do not supply one, not the normal path: a self-minted ID is returned in
	// the action result, which expires after 4 hours, so a caller that relies
	// on it loses the ability to locate the capture in the track after that.
	captureID := inputs.CaptureID
	if captureID == "" {
		captureID = uuid.New().String()
		log.Warnf("pcap: no captureId supplied, generated %s; this capture is only retrievable by an unfiltered networkpcap track query once the action result expires", captureID)
	}

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
