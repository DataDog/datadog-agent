// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
)

// TestNetworkPcapUploader_RealIntake uploads a synthetic pcap with the real
// uploader to a real intake, so the upload wire format can be checked without
// a system-probe. It proves the intake accepts the upload (202), not that the
// attachment reaches storage.
//
// Enable with:
//
//	PCAP_REAL_UPLOAD_TEST=1 \
//	PCAP_REAL_UPLOAD_API_KEY=<real DD_API_KEY, e.g. from `dd-auth --domain dd.datad0g.com --output -- env`> \
//	PCAP_REAL_UPLOAD_SITE=datad0g.com \
//	PCAP_REAL_UPLOAD_INTAKE_HOST=<intake host:port> \
//	dda inv test --targets=./pkg/privateactionrunner/bundles/remoteaction/pcap/...
func TestNetworkPcapUploader_RealIntake(t *testing.T) {
	if os.Getenv("PCAP_REAL_UPLOAD_TEST") != "1" {
		t.Skip("set PCAP_REAL_UPLOAD_TEST=1 to run against a real networkpcap intake")
	}

	apiKey := os.Getenv("PCAP_REAL_UPLOAD_API_KEY")
	require.NotEmpty(t, apiKey, "PCAP_REAL_UPLOAD_API_KEY must be set to a real DD_API_KEY")

	site := os.Getenv("PCAP_REAL_UPLOAD_SITE")
	if site == "" {
		site = "datad0g.com"
	}

	intakeHost := os.Getenv("PCAP_REAL_UPLOAD_INTAKE_HOST")
	require.NotEmpty(t, intakeHost, "PCAP_REAL_UPLOAD_INTAKE_HOST must be set to the intake host:port")

	uploader := newNetworkPcapUploader(&config.Config{
		APIKey:               apiKey,
		DatadogSite:          site,
		NetworkPcapLogsDDURL: intakeHost,
	})

	// Minimal valid pcap: global header only, no packet records.
	syntheticPcap := []byte{
		0xd4, 0xc3, 0xb2, 0xa1, 0x02, 0x00, 0x04, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
	}

	captureID := "uploader-real-test-" + uuid.New().String()
	pcapPath := filepath.Join(t.TempDir(), "synthetic.pcap")
	require.NoError(t, os.WriteFile(pcapPath, syntheticPcap, 0o600))
	err := uploader.Upload(context.Background(), pcapPath, captureID)
	require.NoError(t, err, "expected the real networkpcap intake to accept the zstd multipart upload (202)")

	t.Logf("uploaded capture_id=%s via real networkPcapUploader.Upload to https://%s/api/v2/networkpcap and got 202 — edge-accept confirmed for the real Go code path; S3/Husky storage landing is not independently verifiable with tooling available in this project", captureID, intakeHost)
}
