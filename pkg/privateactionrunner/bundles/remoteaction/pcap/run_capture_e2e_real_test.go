// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	fileupload "github.com/DataDog/datadog-agent/comp/forwarder/fileupload/def"
	fileuploadimpl "github.com/DataDog/datadog-agent/comp/forwarder/fileupload/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// TestRunCapture_RealEndToEnd runs a real capture against a live system-probe
// and uploads it with the real file upload forwarder to a real intake. It proves the intake
// accepts the upload (202), not that the attachment reaches storage.
//
// Enable with:
//
//	PCAP_REAL_E2E_TEST=1 \
//	PCAP_REAL_E2E_API_KEY=<real DD_API_KEY, e.g. from `dd-auth --domain dd.datad0g.com --output -- env`> \
//	PCAP_REAL_E2E_SITE=datad0g.com \
//	PCAP_REAL_E2E_INTAKE_HOST=<intake host:port> \
//	PCAP_REAL_SYSPROBE_SOCKET=/opt/datadog-agent/run/sysprobe.sock \
//	PCAP_REAL_SYSPROBE_IFACE=eth0 \
//	dda inv test --targets=./pkg/privateactionrunner/bundles/remoteaction/pcap/... --build-include=<tags incl. pcap>
//
// while generating traffic (e.g. `ping 8.8.8.8`) on the target interface for
// at least the capture duration below.
func TestRunCapture_RealEndToEnd(t *testing.T) {
	if os.Getenv("PCAP_REAL_E2E_TEST") != "1" {
		t.Skip("set PCAP_REAL_E2E_TEST=1 to run against a live system-probe and a real networkpcap intake")
	}

	apiKey := os.Getenv("PCAP_REAL_E2E_API_KEY")
	require.NotEmpty(t, apiKey, "PCAP_REAL_E2E_API_KEY must be set to a real DD_API_KEY")

	site := os.Getenv("PCAP_REAL_E2E_SITE")
	if site == "" {
		site = "datad0g.com"
	}

	intakeHost := os.Getenv("PCAP_REAL_E2E_INTAKE_HOST")
	require.NotEmpty(t, intakeHost, "PCAP_REAL_E2E_INTAKE_HOST must be set to the intake host:port")

	socketPath := os.Getenv("PCAP_REAL_SYSPROBE_SOCKET")
	if socketPath == "" {
		socketPath = "/opt/datadog-agent/run/sysprobe.sock"
	}
	iface := os.Getenv("PCAP_REAL_SYSPROBE_IFACE")
	if iface == "" {
		iface = "eth0"
	}

	systemProbeConfig := configmock.NewSystemProbe(t)
	systemProbeConfig.SetInTest("system_probe_config.sysprobe_socket", socketPath)

	handler := &RunCaptureHandler{
		uploader: newRealUploader(t, apiKey, site, intakeHost),
		capture:  &socketCaptureTrigger{},
	}

	out, err := handler.capture.Capture(context.Background(), RunCaptureInputs{
		BPFFilter:    "icmp",
		DurationSecs: 3,
		Interface:    iface,
	})
	require.NoError(t, err)
	defer os.Remove(out.PcapPath)
	t.Logf("captured %d packets (%d bytes) in %s -> %s", out.PacketCount, out.FileSizeBytes, out.Duration, out.PcapPath)
	require.Greater(t, out.PacketCount, 0, "expected at least one ICMP packet — is traffic being generated on %s during the capture window?", iface)

	captureID := "e2e-test-" + uuid.New().String()
	err = uploadCapture(context.Background(), handler.uploader, out.PcapPath, captureID)
	require.NoError(t, err, "expected the real networkpcap intake to accept the upload (202); a failure here means the agent-side upload path is broken against a real, reachable intake, independent of any S3/Husky-landing question")

	t.Logf("uploaded capture_id=%s (%d bytes) to https://%s/api/v2/networkpcap and got 202 — this confirms edge-accept only, NOT confirmed storage landing in S3/Husky", captureID, out.FileSizeBytes, intakeHost)
}

// TestRunCapture_RealEndToEndViaRun drives the action through Run() with a
// real task, so input extraction, validation and defaulting are covered too.
// It checks that:
//
//   - the caller's captureId reaches the upload unchanged;
//   - an omitted bpfFilter is accepted and captures everything;
//   - maxPackets bounds the capture.
//
// Enable with the same variables as TestRunCapture_RealEndToEnd. No traffic
// generator is needed: with no filter, ambient host traffic is enough.
func TestRunCapture_RealEndToEndViaRun(t *testing.T) {
	if os.Getenv("PCAP_REAL_E2E_TEST") != "1" {
		t.Skip("set PCAP_REAL_E2E_TEST=1 to run against a live system-probe and a real networkpcap intake")
	}

	apiKey := os.Getenv("PCAP_REAL_E2E_API_KEY")
	require.NotEmpty(t, apiKey, "PCAP_REAL_E2E_API_KEY must be set to a real DD_API_KEY")

	site := os.Getenv("PCAP_REAL_E2E_SITE")
	if site == "" {
		site = "datad0g.com"
	}

	intakeHost := os.Getenv("PCAP_REAL_E2E_INTAKE_HOST")
	require.NotEmpty(t, intakeHost, "PCAP_REAL_E2E_INTAKE_HOST must be set to the intake host:port")

	socketPath := os.Getenv("PCAP_REAL_SYSPROBE_SOCKET")
	if socketPath == "" {
		socketPath = "/opt/datadog-agent/run/sysprobe.sock"
	}
	iface := os.Getenv("PCAP_REAL_SYSPROBE_IFACE")
	if iface == "" {
		iface = "eth0"
	}

	systemProbeConfig := configmock.NewSystemProbe(t)
	systemProbeConfig.SetInTest("system_probe_config.sysprobe_socket", socketPath)

	handler := NewRunCaptureHandler(newRealUploader(t, apiKey, site, intakeHost))

	// Deliberately low so the cap, not the duration, is what ends the capture —
	// otherwise a quiet host would pass without the bound ever being exercised.
	const maxPackets = 100
	captureID := "e2e-run-" + uuid.New().String()

	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]interface{}{
			"captureId":    captureID,
			"durationSecs": 10,
			"interface":    iface,
			"maxPackets":   maxPackets,
			// bpfFilter deliberately absent.
		},
	}

	out, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err, "Run() must accept a task with no bpfFilter")

	res, ok := out.(*RunCaptureResult)
	require.True(t, ok, "expected *RunCaptureResult")

	assert.Equal(t, captureID, res.CaptureID,
		"the caller's captureId must be echoed back, not replaced by a generated one")
	assert.Greater(t, res.PacketCount, 0,
		"expected ambient traffic to be captured with no filter applied")
	assert.LessOrEqual(t, res.PacketCount, maxPackets,
		"maxPackets must bound the capture")

	t.Logf("captured %d packets (%d bytes) in %ds, uploaded capture_id=%s to https://%s/api/v2/networkpcap",
		res.PacketCount, res.FileSizeBytes, res.DurationSecs, res.CaptureID, intakeHost)
}

// newRealUploader returns the real file upload forwarder, sending to
// intakeHost with apiKey.
func newRealUploader(t *testing.T, apiKey, site, intakeHost string) fileupload.Component {
	t.Helper()
	cfg := configcomp.NewMockWithOverrides(t, map[string]interface{}{
		"api_key":                            apiKey,
		"site":                               site,
		"network_pcap.forwarder.logs_dd_url": intakeHost,
	})
	return fileuploadimpl.NewComponent(fileuploadimpl.Requires{Config: cfg}).Comp
}
