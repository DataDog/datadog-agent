// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// asserting Server starts a TLS Server with provided callback function used to perform assertions
// on the contents of the request the server received. If no error is returned server will send standardised
// response OK.
func assertingServer(t *testing.T, onReq func(req *http.Request, reqBody []byte) error) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		assert.NoError(t, onReq(req, body))
		_, err = w.Write([]byte("OK"))
		assert.NoError(t, err)
		req.Body.Close()
	}))
}

func newRequestRecorder(t *testing.T) (req *http.Request, rec *httptest.ResponseRecorder) {
	req, err := http.NewRequest("POST", "/telemetry/proxy/path", strings.NewReader("body"))
	assert.NoError(t, err)
	rec = httptest.NewRecorder()
	return req, rec
}

func recordedResponse(t *testing.T, rec *httptest.ResponseRecorder) string {
	resp := rec.Result()
	responseBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.NoError(t, err)

	return string(responseBody)
}

func TestTelemetryBasicProxyRequest(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("body", string(body), "invalid request body")
		assert.Equal("test_apikey", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal("test_ARN", req.Header.Get("DD-Function-ARN"))
		assert.Equal("/path", req.URL.Path)
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		endpointCalled.Inc()
		return nil
	})

	req, rec := newRequestRecorder(t)
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test_apikey"
	cfg.TelemetryConfig.Enabled = true
	cfg.TelemetryConfig.Endpoints = []*config.Endpoint{{
		APIKey: "test_apikey",
		Host:   srv.URL,
	}}
	cfg.Hostname = "test_hostname"
	cfg.SkipSSLValidation = true
	cfg.DefaultEnv = "test_env"
	cfg.GlobalTags[functionARNKey] = "test_ARN"
	recv := newTestReceiverFromConfig(cfg)
	recv.buildMux().ServeHTTP(rec, req)

	assert.Equal("OK", recordedResponse(t, rec))
	assert.Equal(uint64(1), endpointCalled.Load())

}

func TestTelemetryProxyMultipleEndpoints(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	mainBackend := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))
		assert.Equal("body", string(body), "invalid request body")
		assert.Equal("test_apikey_1", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal("test_ARN", req.Header.Get("DD-Function-ARN"))

		endpointCalled.Add(2)
		return nil
	})
	additionalBackend := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))
		assert.Equal("body", string(body), "invalid request body")
		assert.Equal("test_apikey_2", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal("test_ARN", req.Header.Get("DD-Function-ARN"))

		endpointCalled.Add(3)
		return nil
	})

	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test_apikey_1"
	cfg.TelemetryConfig.Enabled = true
	cfg.TelemetryConfig.Endpoints = []*config.Endpoint{{
		APIKey: "test_apikey_1",
		Host:   mainBackend.URL,
	}, {
		APIKey: "test_apikey_3",
		Host:   "111://malformed_url.example.com",
	}, {
		APIKey: "test_apikey_2",
		Host:   additionalBackend.URL + "/",
	}}
	cfg.Hostname = "test_hostname"
	cfg.SkipSSLValidation = true
	cfg.DefaultEnv = "test_env"
	cfg.GlobalTags[functionARNKey] = "test_ARN"

	req, rec := newRequestRecorder(t)
	recv := newTestReceiverFromConfig(cfg)
	recv.buildMux().ServeHTTP(rec, req)

	assert.Equal("OK", recordedResponse(t, rec))

	// because we use number 2,3 both endpoints must be called to produce 5
	// just counting number of requests could give false results if first endpoint
	// was called twice
	if endpointCalled.Load() != 5 {
		t.Fatalf("calling multiple backends failed")
	}
}

func TestTelemetryConfig(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "api_key"

		req, rec := newRequestRecorder(t)
		recv := newTestReceiverFromConfig(cfg)
		recv.buildMux().ServeHTTP(rec, req)
		result := rec.Result()
		assert.Equal(t, 404, result.StatusCode)
		result.Body.Close()
	})

	t.Run("no-endpoints", func(t *testing.T) {
		cfg := config.New()
		cfg.TelemetryConfig.Enabled = true
		cfg.TelemetryConfig.Endpoints = []*config.Endpoint{{
			APIKey: "api_key",
			Host:   "111://malformed.dd_url.com",
		}}
		req, rec := newRequestRecorder(t)
		recv := newTestReceiverFromConfig(cfg)
		recv.buildMux().ServeHTTP(rec, req)
		result := rec.Result()
		assert.Equal(t, 404, result.StatusCode)
		result.Body.Close()
	})

	t.Run("fallback-endpoint", func(t *testing.T) {
		srv := assertingServer(t, func(req *http.Request, body []byte) error { return nil })
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "api_key"
		cfg.TelemetryConfig.Enabled = true
		cfg.SkipSSLValidation = true
		cfg.TelemetryConfig.Endpoints = []*config.Endpoint{{
			APIKey: "api_key",
			Host:   "111://malformed.dd_url.com",
		}, {
			APIKey: "api_key",
			Host:   srv.URL,
		}}

		req, rec := newRequestRecorder(t)
		recv := newTestReceiverFromConfig(cfg)
		recv.buildMux().ServeHTTP(rec, req)

		assert.Equal(t, "OK", recordedResponse(t, rec))
	})
}
