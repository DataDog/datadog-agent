// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"bytes"
	"compress/gzip"
	"context"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestForwarder(t *testing.T, backend *httptest.Server, timeout ...time.Duration) *forwarder {
	t.Helper()
	cfg := DefaultConfig()
	if len(timeout) > 0 {
		cfg.RequestTimeout = timeout[0]
	}
	cfg.ServerConfig.NetAddr.Endpoint = "127.0.0.1:0"
	set := extension.Settings{ID: component.NewID(component.MustNewType("datadog")), TelemetrySettings: componenttest.NewNopTelemetrySettings()}
	e, err := New(set, cfg, "collector-secret", "datadoghq.com", confighttp.NewDefaultClientConfig())
	require.NoError(t, err)
	f := e.(*forwarder)
	f.backend, err = url.Parse(backend.URL)
	require.NoError(t, err)
	require.NoError(t, f.Start(t.Context(), componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, f.Shutdown(context.Background())) })
	return f
}

func call(t *testing.T, f *forwarder, method, path string, body []byte, headers http.Header) (int, http.Header, []byte) {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Timeout: time.Second}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest(method, "http://"+f.listener.Addr().String()+path, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header = headers
	response, err := client.Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, response.Header, result
}

func TestOpaquePayloadAndAuthentication(t *testing.T) {
	payload := []byte{0, 1, 2, 255, 42}
	expected := []byte{255, 42, 0, 3}
	var requests atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, []string{"collector-secret"}, r.Header.Values("DD-API-KEY"))
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		require.Empty(t, r.Header.Get("DD-APPLICATION-KEY"))
		require.Empty(t, r.Header.Get("X-Workload-Secret"))
		require.Empty(t, r.Header.Get("X-Hop-Only"))
		require.Equal(t, "application/x-protobuf", r.Header.Get("Content-Type"))
		require.Equal(t, "client-runtime", r.Header.Get("DD-Client-Runtime-ID"))
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, payload, data)
		require.Equal(t, "/api/v0.1/configurations", r.URL.Path)
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Header().Add("X-Reply", "one")
		w.Header().Add("X-Reply", "two")
		w.Header().Set("DD-API-KEY", "should-not-reach-sdk")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write(expected)
	}))
	defer backend.Close()
	f := newTestForwarder(t, backend)
	headers := http.Header{"Dd-Api-Key": []string{"sdk-key", "duplicate-key"}, "Authorization": []string{"Bearer sdk-secret"}, "Cookie": []string{"sdk-session"}, "Dd-Application-Key": []string{"app-secret"}, "X-Workload-Secret": []string{"private"}, "Content-Type": []string{"application/x-protobuf"}, "Dd-Client-Runtime-Id": []string{"client-runtime"}, "Connection": []string{"X-Hop-Only"}, "X-Hop-Only": []string{"removed"}}
	status, h, body := call(t, f, "POST", "/api/v0.1/configurations", payload, headers)
	require.Equal(t, http.StatusAccepted, status)
	require.Equal(t, expected, body)
	require.Equal(t, []string{"one", "two"}, h.Values("X-Reply"))
	require.Empty(t, h.Get("DD-API-KEY"))
	require.Equal(t, "no-store", h.Get("Cache-Control"))
	require.EqualValues(t, 1, requests.Load())
}

func TestRedirectIsReturnedWithoutFollowing(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "collector-secret", r.Header.Get("DD-API-KEY"))
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer backend.Close()
	f := newTestForwarder(t, backend)
	status, h, _ := call(t, f, "GET", "/api/v0.1/org", nil, nil)
	require.Equal(t, http.StatusTemporaryRedirect, status)
	require.Equal(t, target.URL, h.Get("Location"))
	require.Zero(t, targetCalls.Load())
}

func TestCompressedResponsePreserved(t *testing.T) {
	var encoded bytes.Buffer
	compressor := gzip.NewWriter(&encoded)
	_, err := compressor.Write([]byte("opaque signed response"))
	require.NoError(t, err)
	require.NoError(t, compressor.Close())
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(encoded.Bytes())
	}))
	defer backend.Close()
	f := newTestForwarder(t, backend)
	status, h, body := call(t, f, "GET", "/api/v0.1/org", nil, http.Header{"Accept-Encoding": []string{"gzip"}})
	require.Equal(t, 200, status)
	require.Equal(t, "gzip", h.Get("Content-Encoding"))
	require.Equal(t, encoded.Bytes(), body)
}

func TestRejectedRequestsNeverReachBackend(t *testing.T) {
	var calls atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer backend.Close()
	f := newTestForwarder(t, backend)
	f.config.ServerConfig.MaxRequestBodySize = 4
	for _, tc := range []struct {
		method, path, body string
		code               int
	}{
		{"GET", "/unexpected", "", 404}, {"POST", "/api/v0.1/org", "", 405}, {"GET", "/api/v0.1/org?x=1", "", 400}, {"POST", "/api/v0.1/configurations", "12345", 413},
	} {
		t.Run(tc.path+tc.method, func(t *testing.T) {
			status, _, _ := call(t, f, tc.method, tc.path, []byte(tc.body), nil)
			require.Equal(t, tc.code, status)
		})
	}
	require.Zero(t, calls.Load())
}

func TestBackendErrorsAreRedacted(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	f := newTestForwarder(t, backend)
	backend.Close()
	status, _, body := call(t, f, "GET", "/api/v0.1/org", nil, nil)
	require.Equal(t, http.StatusBadGateway, status)
	require.NotContains(t, string(body), backend.URL)
	require.NotContains(t, string(body), f.key)
}

func TestShutdownCancelsBackendRequest(t *testing.T) {
	entered := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer backend.Close()
	f := newTestForwarder(t, backend)
	done := make(chan struct{})
	go func() {
		defer close(done)
		client := http.Client{Timeout: time.Second}
		response, err := client.Get("http://" + f.listener.Addr().String() + "/api/v0.1/org")
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("backend was not reached")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, f.Shutdown(ctx))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not finish after shutdown")
	}
}

func TestInvalidSettings(t *testing.T) {
	cfg := DefaultConfig()
	require.NoError(t, ValidateConfig(cfg, "datad0g.com"))
	require.Error(t, ValidateConfig(cfg, "evil.invalid/path"))
	cfg.RequestTimeout = 0
	require.Error(t, ValidateConfig(cfg, "datadoghq.com"))
	require.True(t, strings.HasPrefix(cfg.ServerConfig.NetAddr.Endpoint, "127.0.0.1:"))
}

func TestDeadlineCancelsStalledBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer backend.Close()
	f := newTestForwarder(t, backend, 25*time.Millisecond)
	started := time.Now()
	status, _, _ := call(t, f, "GET", "/api/v0.1/org", nil, nil)
	require.Contains(t, []int{http.StatusServiceUnavailable, http.StatusGatewayTimeout}, status)
	require.Less(t, time.Since(started), time.Second)
}

func TestCompressedRequestUsesCollectorDecompression(t *testing.T) {
	payload := []byte("opaque protobuf payload")
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, payload, body)
		require.Empty(t, r.Header.Get("Content-Encoding"))
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	f := newTestForwarder(t, backend)
	var encoded bytes.Buffer
	compressor := gzip.NewWriter(&encoded)
	_, err := compressor.Write(payload)
	require.NoError(t, err)
	require.NoError(t, compressor.Close())
	status, _, _ := call(t, f, "POST", "/api/v0.1/configurations", encoded.Bytes(), http.Header{"Content-Encoding": []string{"gzip"}, "Content-Type": []string{"application/x-protobuf"}})
	require.Equal(t, http.StatusOK, status)
}
