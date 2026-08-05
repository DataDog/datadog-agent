// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin && !windows && kubeapiserver

package start

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"start"},
		start,
		func() {})
}

func newGlobalParamsTest(t *testing.T) *command.GlobalParams {
	// Because run uses fx.Invoke, demultiplexer, and workloadmeta component are built
	// which lead to build:
	//   - config.Component which requires a valid datadog.yaml
	config := path.Join(t.TempDir(), "datadog.yaml")
	err := os.WriteFile(config, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: config,
	}
}

func TestLoopbackOnly(t *testing.T) {
	handler := loopbackOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "served")
	}))

	tests := []struct {
		name       string
		remoteAddr string
		wantServed bool
	}{
		{"IPv4 loopback", "127.0.0.1:34567", true},
		{"IPv4 loopback range", "127.0.0.5:34567", true},
		{"IPv6 loopback", "[::1]:34567", true},
		{"RemoteAddr without port", "127.0.0.1", true},
		{"private IPv4 off-pod", "10.0.0.5:34567", false},
		{"docker bridge gateway", "172.17.0.1:34567", false},
		{"pod IP", "192.168.49.2:34567", false},
		{"public IPv4", "8.8.8.8:34567", false},
		{"unparseable RemoteAddr", "garbage", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/debug/vars", nil)
			req.RemoteAddr = tc.remoteAddr
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if tc.wantServed {
				assert.Equal(t, http.StatusOK, rec.Code)
				assert.Equal(t, "served", rec.Body.String())
			} else {
				assert.Equal(t, http.StatusNotFound, rec.Code)
				assert.NotContains(t, rec.Body.String(), "served")
			}
		})
	}
}

// TestMetricsMuxRouting mirrors the mux wired in start(): /metrics is public on
// all interfaces, everything under /debug/ is loopback-only.
func TestMetricsMuxRouting(t *testing.T) {
	debug := http.NewServeMux()
	debug.HandleFunc("/debug/vars", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "vars")
	})
	debug.HandleFunc("/debug/pprof/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "pprof")
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "metrics")
	})
	mux.Handle("/debug/", loopbackOnly(debug))

	const offPod = "10.0.0.5:41000"
	const loopback = "127.0.0.1:41000"

	tests := []struct {
		name       string
		path       string
		remoteAddr string
		wantCode   int
		wantBody   string
	}{
		{"metrics reachable off-pod", "/metrics", offPod, http.StatusOK, "metrics"},
		{"metrics reachable loopback", "/metrics", loopback, http.StatusOK, "metrics"},
		{"expvar hidden off-pod", "/debug/vars", offPod, http.StatusNotFound, ""},
		{"expvar served loopback", "/debug/vars", loopback, http.StatusOK, "vars"},
		{"pprof hidden off-pod", "/debug/pprof/heap", offPod, http.StatusNotFound, ""},
		{"pprof served loopback", "/debug/pprof/heap", loopback, http.StatusOK, "pprof"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.RemoteAddr = tc.remoteAddr
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.wantBody != "" {
				assert.Equal(t, tc.wantBody, rec.Body.String())
			}
		})
	}
}
