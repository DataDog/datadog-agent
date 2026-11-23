// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullMux stands in for the main system-probe mux: every route answers 200.
func fullMux() *http.ServeMux {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("POST /packet_capture/capture", ok)
	mux.HandleFunc("/debug/stats", ok)
	mux.HandleFunc("/network_tracer/connections", ok)
	mux.HandleFunc("POST /agent-restart", ok)
	return mux
}

func TestPacketCaptureHandlerServesOnlyCaptureRoutes(t *testing.T) {
	h := newPacketCaptureHandler(fullMux())

	tests := []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/packet_capture/capture", http.StatusOK},
		{http.MethodGet, "/debug/stats", http.StatusNotFound},
		{http.MethodGet, "/network_tracer/connections", http.StatusNotFound},
		{http.MethodPost, "/agent-restart", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}

func TestPacketCaptureHandlerDoesNotEscapePrefix(t *testing.T) {
	h := newPacketCaptureHandler(fullMux())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/packet_capture/../debug/stats", nil))

	// ServeMux redirects to the cleaned path, which this handler does not serve.
	require.GreaterOrEqual(t, rec.Code, 300)
	require.Less(t, rec.Code, 400)
	location := rec.Header().Get("Location")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, location, nil))
	assert.NotEqual(t, http.StatusOK, rec.Code)
}

func TestStartPacketCaptureServer(t *testing.T) {
	// /tmp keeps the path under the unix socket length limit.
	dir, err := os.MkdirTemp("/tmp", "pcapsock")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	socketPath := dir + "/capture.sock"

	startPacketCaptureServer(socketPath, fullMux())

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
	resp, err := client.Post("http://sysprobe/packet_capture/capture", "application/json", nil)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = client.Get("http://sysprobe/debug/stats")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
