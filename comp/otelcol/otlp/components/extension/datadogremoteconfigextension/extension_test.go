// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datadogremoteconfigextension

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/extension"
)

func startProxy(t *testing.T, upstream *httptest.Server, update func(*Config)) *remoteConfigExtension {
	t.Helper()
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.Endpoint = "127.0.0.1:0"
	cfg.Egress.Endpoint = upstream.URL
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: upstream.Certificate().Raw}), 0600))
	cfg.Egress.TLS.CAFile = caFile
	if update != nil {
		update(cfg)
	}
	ext, err := factory.Create(t.Context(), extension.Settings{
		ID: component.NewID(Type), TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)
	e := ext.(*remoteConfigExtension)
	require.NoError(t, e.Start(t.Context(), componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	return e
}

func proxyRequest(t *testing.T, e *remoteConfigExtension, method, path string, body []byte) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, "http://"+e.listener.Addr().String()+path, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("DD-API-KEY", "test-key")
	req.Header.Set("X-Forwarded-Host", "untrusted.example")
	req.Host = "tracer-proxy.invalid"
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	responseBody, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, res.Header, responseBody
}

func TestForwardAgentlessProtocol(t *testing.T) {
	requestBody := []byte{0x00, 0xff, 0x08, 0x01, 0x12, 0x00}
	responseBody := []byte{0xff, 0x00, 0x12, 0x01}
	var calls atomic.Int64
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil || r.Header.Get("DD-API-KEY") != "test-key" || r.Header.Get("X-Forwarded-Host") != "" || r.Host == "tracer-proxy.invalid" {
			http.Error(w, "incorrect forwarded request", http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/api/v0.1/configurations" && (r.Method != http.MethodPost || !bytes.Equal(body, requestBody)) {
			http.Error(w, "incorrect payload", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Content-Type") != "application/x-protobuf" {
			http.Error(w, "incorrect content type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Header().Add("X-Test-Metadata", "first")
		w.Header().Add("X-Test-Metadata", "second")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write(responseBody)
	}))
	defer upstream.Close()
	e := startProxy(t, upstream, nil)
	for _, request := range []struct {
		method, path string
		body         []byte
	}{
		{http.MethodGet, "/api/v0.1/org", nil},
		{http.MethodPost, "/api/v0.1/configurations", requestBody},
		{http.MethodPost, "/api/v0.1/configurations", requestBody},
	} {
		status, headers, body := proxyRequest(t, e, request.method, request.path, request.body)
		require.Equal(t, http.StatusAccepted, status)
		require.Equal(t, responseBody, body)
		require.Equal(t, []string{"first", "second"}, headers.Values("X-Test-Metadata"))
	}
	require.EqualValues(t, 3, calls.Load())
}

func TestRejectUnsupportedRequests(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	e := startProxy(t, upstream, func(c *Config) { c.MaxRequestBodySize = 4 })
	for _, tt := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodGet, "/v0.7/config", "", http.StatusNotFound},
		{http.MethodPost, "/api/v0.1/org", "", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v0.1/configurations", "", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v0.1/configurations?target=other", "", http.StatusBadRequest},
		{http.MethodPost, "/api/v0.1/configurations", "12345", http.StatusRequestEntityTooLarge},
		{http.MethodGet, "/api/v0.1/org/", "", http.StatusNotFound},
	} {
		t.Run(tt.method+tt.path+tt.body, func(t *testing.T) {
			status, headers, _ := proxyRequest(t, e, tt.method, tt.path, []byte(tt.body))
			require.Equal(t, tt.status, status)
			if tt.status == http.StatusMethodNotAllowed {
				require.NotEmpty(t, headers.Get("Allow"))
			}
		})
	}
	require.Zero(t, calls.Load())
}

func TestPreserveUpstreamFailuresAndRedirects(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "60")
				w.Header().Set("Location", "https://must-not-be-contacted.invalid/")
				w.WriteHeader(status)
				_, _ = w.Write([]byte("upstream response"))
			}))
			defer upstream.Close()
			e := startProxy(t, upstream, nil)
			actualStatus, headers, body := proxyRequest(t, e, http.MethodGet, "/api/v0.1/org", nil)
			require.Equal(t, status, actualStatus)
			require.Equal(t, "60", headers.Get("Retry-After"))
			require.Equal(t, "upstream response", string(body))
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTransportErrors(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			settings := componenttest.NewNopTelemetrySettings()
			counter, err := settings.MeterProvider.Meter("test").Int64Counter("test.requests")
			require.NoError(t, err)
			e := &remoteConfigExtension{config: createDefaultConfig().(*Config)}
			handler := e.handler(roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if timeout {
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				return nil, errors.New("secret transport details")
			}), counter)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v0.1/org", nil))
			status := http.StatusBadGateway
			if timeout {
				status = http.StatusGatewayTimeout
			}
			require.Equal(t, status, res.Code)
			require.NotContains(t, res.Body.String(), "secret")
		})
	}
}

func TestShutdownDrainsRequest(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	e := startProxy(t, upstream, nil)
	type result struct {
		response *http.Response
		err      error
	}
	requestDone := make(chan result, 1)
	go func() {
		client := &http.Client{}
		defer client.CloseIdleConnections()
		res, err := client.Get("http://" + e.listener.Addr().String() + "/api/v0.1/org")
		if res != nil {
			_ = res.Body.Close()
		}
		requestDone <- result{res, err}
	}()
	<-started
	stopping := make(chan struct{})
	e.server.RegisterOnShutdown(sync.OnceFunc(func() { close(stopping) }))
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- e.Shutdown(context.Background()) }()
	<-stopping
	close(release)
	requestResult := <-requestDone
	require.NoError(t, requestResult.err)
	require.Equal(t, http.StatusNoContent, requestResult.response.StatusCode)
	require.NoError(t, <-shutdownDone)
}

func TestConfigValidation(t *testing.T) {
	for _, endpoint := range []string{"http://config.datadoghq.com", "https://user:password@example.com", "https://example.com/path", "https://example.com?query", "https://example.com#fragment", ""} {
		cfg := createDefaultConfig().(*Config)
		cfg.Egress.Endpoint = endpoint
		require.Error(t, cfg.Validate())
	}
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, cfg.Validate())
	cfg.Egress.Timeout = 0
	require.Error(t, cfg.Validate())
	cfg = createDefaultConfig().(*Config)
	cfg.MaxRequestBodySize = 0
	require.Error(t, cfg.Validate())
	cfg = createDefaultConfig().(*Config)
	cfg.Endpoint = "invalid"
	require.Error(t, cfg.Validate())
}

func TestTLSListener(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	certDir := t.TempDir()
	certFile, keyFile := filepath.Join(certDir, "cert.pem"), filepath.Join(certDir, "key.pem")
	privateKey, err := x509.MarshalPKCS8PrivateKey(upstream.TLS.Certificates[0].PrivateKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: upstream.Certificate().Raw}), 0600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey}), 0600))
	e := startProxy(t, upstream, func(c *Config) {
		c.TLS = &configtls.ServerConfig{Config: configtls.Config{CertFile: certFile, KeyFile: keyFile}}
	})
	client := upstream.Client()
	defer client.CloseIdleConnections()
	res, err := client.Get("https://" + e.listener.Addr().String() + "/api/v0.1/org")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func TestStartFailureAndShutdown(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Endpoint = "127.0.0.1:0"
	cfg.TLS = &configtls.ServerConfig{Config: configtls.Config{CertFile: "missing-cert", KeyFile: "missing-key"}}
	e := &remoteConfigExtension{config: cfg, settings: extension.Settings{TelemetrySettings: componenttest.NewNopTelemetrySettings()}}
	require.Error(t, e.Start(t.Context(), componenttest.NewNopHost()))
	require.NoError(t, e.Shutdown(t.Context()))
}

func TestCompressedPayloadIsOpaque(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte{0x00, 0xff, 0x08, 0x01})
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || r.Header.Get("Content-Encoding") != "gzip" || !bytes.Equal(body, compressed.Bytes()) {
			http.Error(w, "payload changed", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()
	e := startProxy(t, upstream, nil)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+e.listener.Addr().String()+"/api/v0.1/configurations", bytes.NewReader(compressed.Bytes()))
	require.NoError(t, err)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")
	client := &http.Client{}
	defer client.CloseIdleConnections()
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "gzip", res.Header.Get("Content-Encoding"))
	require.Equal(t, compressed.Bytes(), body)
}

func TestShutdownClosesUpstreamConnection(t *testing.T) {
	closed := make(chan struct{})
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			close(closed)
		}
	}
	upstream.StartTLS()
	defer upstream.Close()
	e := startProxy(t, upstream, nil)
	e.client.Transport.(*http.Transport).IdleConnTimeout = 0
	status, _, _ := proxyRequest(t, e, http.MethodGet, "/api/v0.1/org", nil)
	require.Equal(t, http.StatusNoContent, status)
	require.NoError(t, e.Shutdown(t.Context()))
	<-closed
}
