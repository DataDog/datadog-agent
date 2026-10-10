// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && pcap && cgo

package modules

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleCaptureSendsHeadersBeforeFirstPacket checks that a capture on a
// quiet interface answers within the caller's response-header timeout instead
// of holding the headers until the first packet or the end of the capture.
func TestHandleCaptureSendsHeadersBeforeFirstPacket(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("opening a capture handle requires root")
	}

	srv := httptest.NewServer(http.HandlerFunc(handleCapture))
	t.Cleanup(srv.Close)
	client := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: time.Second}}

	// Nothing listens on the discard port, so the filter matches no traffic.
	body := `{"interface":"lo","bpfFilter":"udp port 9","durationSecs":3}`
	resp, err := client.Post(srv.URL, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "0", resp.Trailer.Get("X-Packet-Count"))
}
