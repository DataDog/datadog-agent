// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package com_datadoghq_remoteaction_pcap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// setupGracePeriod is added to the requested capture duration when bounding
// the HTTP round trip to system-probe, to allow for TC attach/detach and
// response streaming overhead beyond the capture window itself. It is also
// used as the response-header timeout below, since attaching the eBPF TC
// program can itself take several seconds.
const setupGracePeriod = 30 * time.Second

// captureHTTPClient returns an http.Client dedicated to packet_capture
// requests. It deliberately does not reuse sysprobeclient.Get's shared
// client: that client's 5s ResponseHeaderTimeout and 10s overall Timeout are
// sized for quick check-style requests and are too short for a capture,
// which can take several seconds just to attach its eBPF TC program before
// headers are sent, and can legitimately stream for up to
// maxDurationSecs (120s) afterwards.
var captureHTTPClient = funcs.MemoizeArgNoError[string, *http.Client](func(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:          2,
			IdleConnTimeout:       30 * time.Second,
			DialContext:           sysprobeclient.DialContextFunc(socketPath),
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: setupGracePeriod,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}
})

// captureRequest is the JSON body sent to system-probe's packet_capture
// module. Field names and shape must match cmd/system-probe/modules/packet_capture.go.
type captureRequest struct {
	Interface    string `json:"interface,omitempty"`
	BPFFilter    string `json:"bpfFilter,omitempty"`
	DurationSecs int    `json:"durationSecs"`
	MaxPackets   uint64 `json:"maxPackets,omitempty"`
	MaxBytes     uint64 `json:"maxBytes,omitempty"`
	SnapLen      uint32 `json:"snapLen,omitempty"`
	HeaderOnly   bool   `json:"headerOnly,omitempty"`
}

// socketCaptureTrigger triggers captures via system-probe's packet_capture
// module over its unix domain socket.
type socketCaptureTrigger struct{}

func newCaptureTrigger() captureTrigger {
	return &socketCaptureTrigger{}
}

// Capture triggers a packet capture on system-probe over its unix socket
// and streams the resulting PCAP data to a local temp file.
func (*socketCaptureTrigger) Capture(ctx context.Context, inputs RunCaptureInputs) (packetCount int, fileSizeBytes int64, actualDuration time.Duration, pcapPath string, err error) {
	socketPath := pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")
	if socketPath == "" {
		return 0, 0, 0, "", errors.New("system-probe socket path not configured (system_probe_config.sysprobe_socket)")
	}

	// HeaderOnly is unconditional, not sourced from inputs: the settled snap-length
	// design (NET/7010419694) has no payload opt-in, so this trigger never sends a
	// request that could return full packet payloads.
	reqBody, err := json.Marshal(captureRequest{
		Interface:    inputs.Interface,
		BPFFilter:    inputs.BPFFilter,
		DurationSecs: inputs.DurationSecs,
		MaxPackets:   uint64(inputs.MaxPackets),
		MaxBytes:     uint64(inputs.MaxBytes),
		SnapLen:      uint32(inputs.SnapLen),
		HeaderOnly:   true,
	})
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("marshalling capture request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(inputs.DurationSecs)*time.Second+setupGracePeriod)
	defer cancel()

	url := sysprobeclient.ModuleURL(sysconfig.PacketCaptureModule, "/capture")
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	resp, err := captureHTTPClient(socketPath).Do(req)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("calling system-probe packet_capture module: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := sysprobeclient.ReadAllResponseBody(resp)
		return 0, 0, 0, "", fmt.Errorf("packet_capture request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	tmpFile, err := os.CreateTemp(os.TempDir(), "dd-pcap-*.pcap")
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err = io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return 0, 0, 0, "", fmt.Errorf("writing pcap data: %w", err)
	}

	actualDuration = time.Since(startTime)

	if err = tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return 0, 0, 0, "", fmt.Errorf("flushing pcap file: %w", err)
	}

	fi, err := tmpFile.Stat()
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return 0, 0, 0, "", fmt.Errorf("stat pcap file: %w", err)
	}

	if err = tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return 0, 0, 0, "", fmt.Errorf("closing pcap file: %w", err)
	}

	// The trailer is only populated by the net/http client once the body has
	// been fully read (above), and reports the packet_capture module's final
	// stats for this capture.
	if v := resp.Trailer.Get("X-Packet-Count"); v != "" {
		if n, parseErr := strconv.Atoi(v); parseErr == nil {
			packetCount = n
		}
	}

	return packetCount, fi.Size(), actualDuration, tmpPath, nil
}
