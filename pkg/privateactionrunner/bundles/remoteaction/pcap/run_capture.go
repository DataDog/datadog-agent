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
	// defaultSnapLen matches system-probe's ceiling; system-probe trims every
	// packet to its headers regardless.
	defaultSnapLen = 128
	// defaultMaxPackets and defaultMaxBytes bound an otherwise unbounded capture.
	// The constraint they serve is not the pipeline but the usability of the
	// artefact: Wireshark's limit is packet count, not file size, and it costs
	// roughly 0.5-1 KB of RAM per packet for dissection state. Header-only
	// capture packs ~9x more packets into a megabyte than an ordinary
	// full-packet capture, so file-size intuitions from normal pcaps do not
	// transfer.
	//
	// A busy host produces 2-3M packets in 30s, and 2M covers most of that while
	// staying openable: ~2 GB of Wireshark RAM, slow to load but usable. Beyond
	// that, minutes-to-load and re-dissection on every filter change; 9M swaps or
	// OOMs a 16 GB laptop. The busiest hosts (~6M packets in 30s unfiltered) are
	// still truncated, so a full-duration window there needs a BPF filter, not a
	// higher cap.
	defaultMaxPackets = 2_000_000
	// defaultMaxBytes is the largest upload we are willing to ship. It backstops
	// defaultMaxPackets rather than binding first: at ~76 bytes per header-only
	// record, 2M packets is ~152 MB, just under. It takes effect when packets are
	// larger than the header-only assumption (e.g. a raised snapLen), which is
	// exactly the case where a packet count is the wrong guardrail.
	//
	// Sizing note: the intake's real ceiling is a 30s edge timeout rather than a
	// content-length limit. Header-only pcaps compress ~4x, so 160 MiB is ~40 MB
	// on the wire and assumes the host can sustain ~11 Mbit/s to the intake. A
	// slow uplink fails on time, not on size.
	defaultMaxBytes = 160 * 1024 * 1024
	minDurationSecs = 1
	maxDurationSecs = 120
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
	// PacketsDropped counts packets the kernel discarded because the capture
	// could not keep up. Non-zero means the capture is incomplete.
	PacketsDropped int `json:"packetsDropped"`
	// HeadersTruncated counts packets whose headers did not fit in snapLen.
	HeadersTruncated int `json:"headersTruncated"`
	// CaptureErrors counts packets that could not be read or written.
	CaptureErrors int `json:"captureErrors"`
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
	// system-probe treats "" as match-all rather than as an error.

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

	out, err := h.capture.Capture(ctx, inputs)
	if err != nil {
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	if out.PcapPath != "" {
		defer os.Remove(out.PcapPath)

		if err := h.sendCapture(ctx, out.PcapPath, captureID); err != nil {
			return nil, fmt.Errorf("sending capture %s to event platform: %w", captureID, err)
		}
	}

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
