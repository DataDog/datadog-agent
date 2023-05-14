// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

func testCfg(serverUrl string) *config.AgentConfig {
	cfg := config.New()
	cfg.TelemetryConfig.Endpoints[0].Host = serverUrl
	cfg.TelemetryConfig.Enabled = true
	return cfg
}

func TestTelemetryDisabled(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cfg := testCfg(server.URL)
	cfg.TelemetryConfig.Enabled = false

	collector := NewCollector(cfg)

	server.assertReq = func(req *http.Request) {
		t.Fail()
	}
	collector.SendStartupError(GenericError, fmt.Errorf(""))
	collector.SendStartupSuccess()
}

func TestTelemetryPath(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cfg := testCfg(server.URL)
	collector := NewCollector(cfg)

	var reqCount int
	var path string
	server.assertReq = func(req *http.Request) {
		reqCount += 1
		path = req.URL.Path
	}

	collector.SendStartupError(GenericError, fmt.Errorf(""))
	collector.SendStartupSuccess()

	assert.Equal(t, 1, reqCount)
	assert.Equal(t, "/api/v2/apmtelemetry", path)
}

func TestNoSuccessAfterError(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cfg := testCfg(server.URL)
	collector := NewCollector(cfg)

	var eventName string
	var reqCount int

	server.assertReq = func(req *http.Request) {
		reqCount += 1
		body, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		ev := OnboardingEvent{}
		err = json.Unmarshal(body, &ev)
		assert.NoError(t, err)
		eventName = ev.Payload.EventName
	}

	collector.SendStartupError(GenericError, fmt.Errorf(""))
	collector.SendStartupSuccess()

	assert.Equal(t, 1, reqCount)
	assert.Equal(t, "agent.startup.error", eventName)
}

func TestErrorAfterSuccess(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cfg := testCfg(server.URL)
	collector := NewCollector(cfg)

	var eventNames []string
	var reqCount int

	server.assertReq = func(req *http.Request) {
		reqCount += 1
		body, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		ev := OnboardingEvent{}
		err = json.Unmarshal(body, &ev)
		assert.NoError(t, err)
		eventNames = append(eventNames, ev.Payload.EventName)
	}

	collector.SendStartupSuccess()
	collector.SendStartupError(GenericError, fmt.Errorf(""))

	assert.Equal(t, 2, reqCount)
	assert.Equal(t, "agent.startup.success", eventNames[0])
	assert.Equal(t, "agent.startup.error", eventNames[1])
}

func TestDualShipping(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	server2 := newTestServer()
	defer server2.Close()

	cfg := testCfg(server.URL)
	cfg.TelemetryConfig.Endpoints = append(cfg.TelemetryConfig.Endpoints, &config.Endpoint{Host: server2.URL})

	collector := NewCollector(cfg)

	var body, body2 []byte
	var reqCount, reqCount2 int

	server.assertReq = func(req *http.Request) {
		reqCount += 1
		b, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		body = b
	}
	server2.assertReq = func(req *http.Request) {
		reqCount2 += 1
		b, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		body2 = b
	}

	collector.SendStartupSuccess()

	assert.Equal(t, 1, reqCount)
	assert.Equal(t, reqCount, reqCount2)
	assert.Equal(t, body, body2)
}

type testServer struct {
	server    *httptest.Server
	URL       string
	assertReq func(*http.Request)
}

func newTestServer() *testServer {
	srv := &testServer{}
	srv.server = httptest.NewServer(srv)
	srv.URL = srv.server.URL
	return srv
}

// ServeHTTP responds based on the request body.
func (ts *testServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if ts.assertReq != nil {
		ts.assertReq(req)
	}
	_, err := io.ReadAll(req.Body)
	if err != nil {
		panic(fmt.Sprintf("error reading request body: %v", err))
	}
	req.Body.Close()
	w.WriteHeader(202)
}

// Close closes the underlying http.Server.
func (ts *testServer) Close() { ts.server.Close() }
