// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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

	packetCount, fileSizeBytes, actualDuration, pcapPath, err := doCapture(ctx, inputs)
	if err != nil {
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	actualSecs := int(actualDuration.Round(time.Second).Seconds())

	var attachmentKey string
	if pcapPath != "" {
		defer os.Remove(pcapPath)
		orgID := task.Data.Attributes.OrgId
		attachmentKey, err = h.uploadPcap(ctx, captureID, pcapPath, inputs, orgID)
		if err != nil {
			return nil, fmt.Errorf("upload failed: %w", err)
		}
	}

	return &RunCaptureResult{
		CaptureID:     captureID,
		PacketCount:   packetCount,
		FileSizeBytes: fileSizeBytes,
		DurationSecs:  actualSecs,
		AttachmentKey: attachmentKey,
	}, nil
}

// pcapEventMetadata is the JSON metadata sent alongside the pcap binary.
type pcapEventMetadata struct {
	CaptureID string `json:"capture_id"`
	Hostname  string `json:"hostname,omitempty"`
	Interface string `json:"interface,omitempty"`
	Filter    string `json:"bpf_filter"`
	SnapLen   int    `json:"snap_len"`
	Source    string `json:"ddsource"`
}

// uploadPcap sends the captured pcap file to the EvP intake via multipart POST.
func (h *RunCaptureHandler) uploadPcap(ctx context.Context, captureID string, pcapPath string, inputs RunCaptureInputs, orgID int64) (string, error) {
	cfg := pkgconfigsetup.Datadog()
	apiKey := cfg.GetString("api_key")
	if apiKey == "" {
		return "", errors.New("DD API key not configured")
	}

	site := cfg.GetString("site")
	if site == "" {
		site = "datadoghq.com"
	}

	hostname, _ := os.Hostname()
	meta := pcapEventMetadata{
		CaptureID: captureID,
		Hostname:  hostname,
		Interface: inputs.Interface,
		Filter:    inputs.BPFFilter,
		SnapLen:   inputs.SnapLen,
		Source:    "pcap",
	}

	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshalling metadata: %w", err)
	}

	pcapFile, err := os.Open(pcapPath)
	if err != nil {
		return "", fmt.Errorf("opening pcap file: %w", err)
	}
	defer pcapFile.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	eventPart, err := writer.CreateFormField("event")
	if err != nil {
		return "", fmt.Errorf("creating event form field: %w", err)
	}
	if _, err := eventPart.Write(metaBytes); err != nil {
		return "", fmt.Errorf("writing event metadata: %w", err)
	}

	pcapPart, err := writer.CreateFormFile("pcap_data", captureID+".pcap.gz")
	if err != nil {
		return "", fmt.Errorf("creating pcap form file: %w", err)
	}

	gzWriter := gzip.NewWriter(pcapPart)
	if _, err := io.Copy(gzWriter, pcapFile); err != nil {
		gzWriter.Close()
		return "", fmt.Errorf("compressing pcap data: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return "", fmt.Errorf("closing gzip writer: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("closing multipart writer: %w", err)
	}

	intakeURL := fmt.Sprintf("https://http-intake.logs.%s/v2/track/pcap/org/%d", site, orgID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, intakeURL, &body)
	if err != nil {
		return "", fmt.Errorf("creating HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("DD-EVP-ORIGIN", "datadog-agent")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending pcap to intake: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("intake returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return captureID, nil
}
