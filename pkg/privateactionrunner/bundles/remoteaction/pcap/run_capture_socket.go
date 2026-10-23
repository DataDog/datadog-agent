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
	"net"
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
// the HTTP round trip to system-probe, to allow for capture setup/teardown and
// response streaming overhead beyond the capture window itself. It is also
// used as the response-header timeout below.
const setupGracePeriod = 30 * time.Second

// captureHTTPClient returns an http.Client for capture requests. The shared
// system-probe client's timeouts suit quick requests, not a response that
// streams for up to maxDurationSecs.
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
}

// captureSocketPath returns system-probe's capture-only socket when one is
// configured, so the runner does not need the main system-probe socket, and
// the main socket otherwise.
func captureSocketPath() string {
	path, _ := captureSocket()
	return path
}

// captureSocket returns the socket captureSocketPath picks and which one it
// is: socketCapture, socketMain, or socketNone when neither is configured.
func captureSocket() (string, string) {
	cfg := pkgconfigsetup.SystemProbe()
	if p := cfg.GetString("packet_capture.socket"); p != "" {
		return p, socketCapture
	}
	if p := cfg.GetString("system_probe_config.sysprobe_socket"); p != "" {
		return p, socketMain
	}
	return "", socketNone
}

// socketCaptureTrigger triggers captures via system-probe's packet_capture
// module over its unix domain socket.
type socketCaptureTrigger struct{}

func newCaptureTrigger() captureTrigger {
	return &socketCaptureTrigger{}
}

// Capture triggers a packet capture on system-probe over its unix socket
// and streams the resulting PCAP data to a local temp file. Errors are
// tagged with their outcome reason; untagged ones are capture errors.
func (*socketCaptureTrigger) Capture(ctx context.Context, inputs RunCaptureInputs) (out captureOutcome, err error) {
	socketPath, socket := captureSocket()
	defer func() { out.Socket = socket }()
	if socketPath == "" {
		return captureOutcome{}, withReason(reasonSocketUnreachable, errors.New("system-probe socket path not configured (packet_capture.socket or system_probe_config.sysprobe_socket)"))
	}

	reqBody, err := json.Marshal(captureRequest{
		Interface:    inputs.Interface,
		BPFFilter:    inputs.BPFFilter,
		DurationSecs: inputs.DurationSecs,
		MaxPackets:   uint64(inputs.MaxPackets),
		MaxBytes:     uint64(inputs.MaxBytes),
		SnapLen:      uint32(inputs.SnapLen),
	})
	if err != nil {
		return captureOutcome{}, fmt.Errorf("marshalling capture request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(inputs.DurationSecs)*time.Second+setupGracePeriod)
	defer cancel()

	url := sysprobeclient.ModuleURL(sysconfig.PacketCaptureModule, "/capture")
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return captureOutcome{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	resp, err := captureHTTPClient(socketPath).Do(req)
	if err != nil {
		err = fmt.Errorf("calling system-probe packet_capture module: %w", err)
		// A failed dial means system-probe could not be reached at all; any
		// later failure happened once it had the request.
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "dial" {
			return captureOutcome{}, withReason(reasonSocketUnreachable, err)
		}
		return captureOutcome{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := sysprobeclient.ReadAllResponseBody(resp)
		return captureOutcome{}, withReason(statusReason(resp.StatusCode), fmt.Errorf("packet_capture request failed: status=%d body=%s", resp.StatusCode, string(body)))
	}

	tmpFile, err := os.CreateTemp(os.TempDir(), "dd-pcap-*.pcap")
	if err != nil {
		return captureOutcome{}, fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err = io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return captureOutcome{}, fmt.Errorf("writing pcap data: %w", err)
	}

	// system-probe commits the HTTP 200 and starts streaming the PCAP body
	// before capturer.Start runs, so a capture that failed to start, or that
	// hit packet read/write errors mid-capture, is reported only through this
	// trailer (populated once the body above has been fully read) rather than
	// the status code. Treat any reported error as a failed capture instead of
	// uploading a possibly-empty or partial artifact as if it succeeded.
	if captureErrs := trailerInt(resp, "X-Capture-Errors"); captureErrs > 0 {
		tmpFile.Close()
		os.Remove(tmpPath)
		return captureOutcome{}, fmt.Errorf("packet_capture reported %d capture error(s)", captureErrs)
	}

	actualDuration := time.Since(startTime)

	if err = tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return captureOutcome{}, fmt.Errorf("flushing pcap file: %w", err)
	}

	fi, err := tmpFile.Stat()
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return captureOutcome{}, fmt.Errorf("stat pcap file: %w", err)
	}

	if err = tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return captureOutcome{}, fmt.Errorf("closing pcap file: %w", err)
	}

	// The trailer is only populated by the net/http client once the body has
	// been fully read (above), and reports the packet_capture module's final
	// stats for this capture.
	return captureOutcome{
		PacketCount:      trailerInt(resp, "X-Packet-Count"),
		PacketsDropped:   trailerInt(resp, "X-Packets-Dropped"),
		HeadersTruncated: trailerInt(resp, "X-Headers-Truncated"),
		Errors:           trailerInt(resp, "X-Capture-Errors"),
		FileSizeBytes:    fi.Size(),
		Duration:         actualDuration,
		PcapPath:         tmpPath,
	}, nil
}

// trailerInt reads a numeric trailer, treating a missing or malformed one as 0.
func trailerInt(resp *http.Response, key string) int {
	n, _ := strconv.Atoi(resp.Trailer.Get(key))
	return n
}

// statusReason maps a non-200 status from system-probe's packet_capture
// module to an outcome reason. System-probe does not serialise captures
// today; 409 and 429 are mapped to busy should it start doing so.
func statusReason(code int) string {
	switch {
	case code == http.StatusConflict, code == http.StatusTooManyRequests:
		return reasonBusy
	case code >= 400 && code < 500:
		return reasonBadRequest
	default:
		return reasonCaptureError
	}
}
