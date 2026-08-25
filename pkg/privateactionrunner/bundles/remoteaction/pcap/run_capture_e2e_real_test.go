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
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
)

// TestRunCapture_RealEndToEnd exercises the *full* runCapture pipeline: a
// real capture against a live system-probe (as in
// TestSocketCaptureTrigger_RealSystemProbe), followed by a real
// networkPcapUploader.Upload of the resulting pcap bytes to a real,
// reachable "networkpcap" EVP intake using real Datadog credentials.
//
// This closes the gap that TestSocketCaptureTrigger_RealSystemProbe leaves:
// that test only proves the capture leg works; it never calls Upload, so it
// never proves a capture actually reaches (and is accepted by) the event
// platform. This test proves the latter — 202 Accepted from the real
// intake. It does NOT independently confirm the resulting attachment lands
// in S3/Husky: that requires storage-side access this harness does not
// have. Treat a passing run as "edge-accepted", not "storage-confirmed".
//
// As of 2026-08-18, "pcap-intake.<site>" is not yet routed in staging, so
// PCAP_REAL_E2E_INTAKE_HOST must be set to a currently-routed override (e.g.
// "cws-intake.datad0g.com:443") via network_pcap.logs_dd_url. See
// ai-investigations/remote-pcap/notes.md and agent-uploader-implementation.md.
//
// Enable with:
//
//	PCAP_REAL_E2E_TEST=1 \
//	PCAP_REAL_E2E_API_KEY=<real DD_API_KEY, e.g. from `dd-auth --domain dd.datad0g.com --output -- env`> \
//	PCAP_REAL_E2E_SITE=datad0g.com \
//	PCAP_REAL_E2E_INTAKE_HOST=cws-intake.datad0g.com:443 \
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
	require.NotEmpty(t, intakeHost, "PCAP_REAL_E2E_INTAKE_HOST must be set to a currently-routed host:port override, e.g. cws-intake.datad0g.com:443 (pcap-intake.<site> is not yet routed)")

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
		uploader: newNetworkPcapUploader(&config.Config{
			APIKey:               apiKey,
			DatadogSite:          site,
			NetworkPcapLogsDDURL: intakeHost,
		}),
		capture: &socketCaptureTrigger{},
	}

	packetCount, fileSizeBytes, actualDuration, pcapPath, err := handler.capture.Capture(context.Background(), RunCaptureInputs{
		BPFFilter:    "icmp",
		DurationSecs: 3,
		Interface:    iface,
	})
	require.NoError(t, err)
	defer os.Remove(pcapPath)
	t.Logf("captured %d packets (%d bytes) in %s -> %s", packetCount, fileSizeBytes, actualDuration, pcapPath)
	require.Greater(t, packetCount, 0, "expected at least one ICMP packet — is traffic being generated on %s during the capture window?", iface)

	captureID := "e2e-test-" + uuid.New().String()
	pcapBytes, err := os.ReadFile(pcapPath)
	require.NoError(t, err)

	err = handler.uploader.Upload(context.Background(), pcapBytes, captureID)
	require.NoError(t, err, "expected the real networkpcap intake to accept the upload (202); a failure here means the agent-side upload path is broken against a real, reachable intake, independent of any S3/Husky-landing question")

	t.Logf("uploaded capture_id=%s (%d bytes) to https://%s/api/v2/networkpcap and got 202 — this confirms edge-accept only, NOT confirmed storage landing in S3/Husky", captureID, len(pcapBytes), intakeHost)
}
