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

//nolint:revive // TODO(TEL) Fix revive linter
func testCfg(serverUrl string) *config.AgentConfig {
	cfg := config.New()
	cfg.TelemetryConfig.Endpoints[0].Host = serverUrl
	cfg.TelemetryConfig.Enabled = true
	return cfg
}

func TestSendFirstTrace(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cfg := testCfg(server.URL)
	cfg.InstallSignature = config.InstallSignatureConfig{
		Found:       true,
		InstallID:   "foobar",
		InstallTime: 1,
		InstallType: "manual",
	}

	collector := NewCollector(cfg)

	var events []OnboardingEvent
	server.assertReq = assertReqGetEvent(t, &events)
	if !collector.SentFirstTrace() {
		collector.SendFirstTrace()
	}

	assert.True(t, collector.SentFirstTrace(), true)
	assert.Len(t, events, 1)
	assert.Equal(t, events[0].Payload.Tags.InstallID, "foobar")
	assert.Equal(t, events[0].Payload.Tags.InstallTime, int64(1))
	assert.Equal(t, events[0].Payload.Tags.InstallType, "manual")
}

func TestSendFirstTraceSignatureNotFound(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cfg := testCfg(server.URL)
	cfg.InstallSignature = config.InstallSignatureConfig{
		Found:       false,
		InstallTime: 1,
	}

	collector := NewCollector(cfg)

	var events []OnboardingEvent
	server.assertReq = assertReqGetEvent(t, &events)
	if !collector.SentFirstTrace() {
		collector.SendFirstTrace()
	}

	assert.True(t, collector.SentFirstTrace(), true)
	assert.Len(t, events, 1)
	assert.Equal(t, events[0].Payload.Tags.InstallID, "")
	assert.Equal(t, events[0].Payload.Tags.InstallTime, int64(0))
	assert.Equal(t, events[0].Payload.Tags.InstallType, "")
}

func TestSendFirstTraceError(t *testing.T) {
	server := newTestServer()
	server.statusCode = 500
	defer server.Close()

	cfg := testCfg(server.URL)
	cfg.InstallSignature = config.InstallSignatureConfig{
		Found:       false,
		InstallTime: 1,
	}

	collector := NewCollector(cfg)

	var events []OnboardingEvent
	server.assertReq = assertReqGetEvent(t, &events)
	for i := 0; i < 5; i++ {
		assert.False(t, collector.SentFirstTrace())
		collector.SendFirstTrace()
	}
	if !collector.SentFirstTrace() {
		collector.SendFirstTrace()
		assert.True(t, collector.SentFirstTrace())
	}
	assert.True(t, collector.SentFirstTrace(), true)
	assert.Len(t, events, 5)
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
		//nolint:revive // TODO(TEL) Fix revive linter
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

	var events []OnboardingEvent
	server.assertReq = assertReqGetEvent(t, &events)

	collector.SendStartupError(GenericError, fmt.Errorf(""))
	collector.SendStartupSuccess()

	assert.Len(t, events, 1)
	assert.Equal(t, "agent.startup.error", events[0].Payload.EventName)
}

func TestErrorAfterSuccess(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cfg := testCfg(server.URL)
	collector := NewCollector(cfg)

	var events []OnboardingEvent
	server.assertReq = assertReqGetEvent(t, &events)

	collector.SendStartupSuccess()
	collector.SendStartupError(GenericError, fmt.Errorf(""))

	assert.Len(t, events, 2)
	assert.Equal(t, "agent.startup.success", events[0].Payload.EventName)
	assert.Equal(t, "agent.startup.error", events[1].Payload.EventName)
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
		//nolint:revive // TODO(TEL) Fix revive linter
		reqCount += 1
		b, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		body = b
	}
	server2.assertReq = func(req *http.Request) {
		//nolint:revive // TODO(TEL) Fix revive linter
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
	server     *httptest.Server
	URL        string
	assertReq  func(*http.Request)
	statusCode int
}

func newTestServer() *testServer {
	srv := &testServer{
		statusCode: 202,
	}
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
	w.WriteHeader(ts.statusCode)
}

// Close closes the underlying http.Server.
func (ts *testServer) Close() { ts.server.Close() }

func assertReqGetEvent(t *testing.T, events *[]OnboardingEvent) func(*http.Request) {
	return func(req *http.Request) {
		body, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		ev := OnboardingEvent{}
		err = json.Unmarshal(body, &ev)
		assert.NoError(t, err)
		*events = append(*events, ev)
	}
}
