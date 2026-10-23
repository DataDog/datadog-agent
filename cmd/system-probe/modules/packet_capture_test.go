// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && pcap && cgo

package modules

import (
	"context"
	"io"
	"net"
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

// TestHandleCaptureFlushDoesNotRaceWithPackets checks that the handler's
// initial flush is serialized with the drain loop's packet writes. Run with
// -race: on a busy interface the first packets are written while the handler
// is still flushing the response headers.
func TestHandleCaptureFlushDoesNotRaceWithPackets(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("opening a capture handle requires root")
	}

	conn, err := net.Dial("udp", "127.0.0.1:9")
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		payload := []byte("pcap-race")
		for ctx.Err() == nil {
			_, _ = conn.Write(payload)
		}
	}()

	srv := httptest.NewServer(http.HandlerFunc(handleCapture))
	t.Cleanup(srv.Close)

	body := `{"interface":"lo","bpfFilter":"udp port 9","durationSecs":2}`
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.NotEqual(t, "0", resp.Trailer.Get("X-Packet-Count"))
}

func TestBoundRequest(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		in                   captureRequest
		wantErr              bool
		wantPackets, wantMax uint64
	}{
		{name: "zero duration", in: captureRequest{DurationSecs: 0}, wantErr: true},
		{name: "duration above the limit", in: captureRequest{DurationSecs: maxDurationSecs + 1}, wantErr: true},
		{name: "duration that would overflow", in: captureRequest{DurationSecs: 1 << 40}, wantErr: true},
		{name: "unset caps use the defaults", in: captureRequest{DurationSecs: 5}, wantPackets: defaultMaxPackets, wantMax: defaultMaxBytes},
		{name: "larger caps are lowered", in: captureRequest{DurationSecs: 5, MaxPackets: 1e12, MaxBytes: 1e12}, wantPackets: defaultMaxPackets, wantMax: defaultMaxBytes},
		{name: "smaller caps are kept", in: captureRequest{DurationSecs: maxDurationSecs, MaxPackets: 10, MaxBytes: 1000}, wantPackets: 10, wantMax: 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.in
			err := boundRequest(&r)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantPackets, r.MaxPackets)
			assert.Equal(t, tc.wantMax, r.MaxBytes)
		})
	}
}

type deadlineWriter struct {
	http.ResponseWriter
	deadline time.Time
}

func (d *deadlineWriter) SetWriteDeadline(t time.Time) error {
	d.deadline = t
	return nil
}

func TestSetWriteDeadline(t *testing.T) {
	w := &deadlineWriter{ResponseWriter: httptest.NewRecorder()}
	deadline := time.Now().Add(time.Minute)
	setWriteDeadline(w, deadline)
	assert.Equal(t, deadline, w.deadline)

	// A writer without deadline support is left as is.
	setWriteDeadline(httptest.NewRecorder(), deadline)
}
