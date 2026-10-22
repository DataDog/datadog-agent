// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fileuploadimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	fileupload "github.com/DataDog/datadog-agent/comp/forwarder/fileupload/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestUpload_RealIntake uploads a synthetic pcap to a real networkpcap
// intake, so the wire format can be checked without a system-probe. It proves
// the intake accepts the upload (202), not that the attachment reaches storage.
//
// Enable with:
//
//	FILEUPLOAD_REAL_TEST=1 \
//	FILEUPLOAD_REAL_API_KEY=<real DD_API_KEY, e.g. from `dd-auth --domain dd.datad0g.com --output -- env`> \
//	FILEUPLOAD_REAL_SITE=datad0g.com \
//	FILEUPLOAD_REAL_INTAKE_HOST=<intake host:port, optional> \
//	dda inv test --targets=./comp/forwarder/fileupload/impl/...
func TestUpload_RealIntake(t *testing.T) {
	if os.Getenv("FILEUPLOAD_REAL_TEST") != "1" {
		t.Skip("set FILEUPLOAD_REAL_TEST=1 to run against a real networkpcap intake")
	}

	apiKey := os.Getenv("FILEUPLOAD_REAL_API_KEY")
	require.NotEmpty(t, apiKey, "FILEUPLOAD_REAL_API_KEY must be set to a real DD_API_KEY")
	site := os.Getenv("FILEUPLOAD_REAL_SITE")
	if site == "" {
		site = "datad0g.com"
	}

	cfg := configmock.New(t)
	cfg.SetInTest("api_key", apiKey)
	cfg.SetInTest("site", site)
	if host := os.Getenv("FILEUPLOAD_REAL_INTAKE_HOST"); host != "" {
		cfg.SetInTest("network_pcap.forwarder.logs_dd_url", host)
	}
	u := newUploader(cfg, getTracks())

	// Minimal valid pcap: global header only, no packet records.
	syntheticPcap := []byte{
		0xd4, 0xc3, 0xb2, 0xa1, 0x02, 0x00, 0x04, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
	}
	captureID := "fileupload-real-test-" + uuid.New().String()
	event, err := json.Marshal(map[string]interface{}{
		"ddsource":   "networkpcap",
		"capture_id": captureID,
		"timestamp":  time.Now().UnixMilli(),
	})
	require.NoError(t, err)

	err = u.Upload(context.Background(), fileupload.UploadRequest{
		Track: fileupload.TrackNetworkPcap,
		Event: event,
		Attachments: []fileupload.Attachment{{
			Name:     "pcap",
			Filename: "capture.pcap",
			Open: func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(syntheticPcap)), nil
			},
		}},
	})
	require.NoError(t, err, "expected the real networkpcap intake to accept the zstd multipart upload (202)")
	t.Logf("uploaded capture_id=%s and got 202", captureID)
}
