// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/mocktracer"
)

func TestLeaderForwarder_Forward_NilProxy_Span(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	lf := NewLeaderForwarder(5005, 10)

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	lf.Forward(rw, req)

	assert.Equal(t, http.StatusServiceUnavailable, rw.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.leader_forwarder.forward", span.OperationName())
	assert.Equal(t, false, span.Tag("forward.loop_detected"))
	assert.Equal(t, false, span.Tag("forward.proxy_available"))
}

func TestLeaderForwarder_Forward_LoopDetection_Span(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	leaderServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer leaderServer.Close()

	port := leaderServer.Listener.Addr().(*net.TCPAddr).Port
	lf := NewLeaderForwarder(port, 10)
	lf.SetLeaderIP("127.0.0.1")

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	req.Header.Set("X-DCA-Follower-Forwarded", "true")

	lf.Forward(rw, req)

	assert.Equal(t, http.StatusLoopDetected, rw.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.leader_forwarder.forward", span.OperationName())
	assert.Equal(t, true, span.Tag("forward.loop_detected"))
	assert.Equal(t, false, span.Tag("forward.proxy_available"))
}

func TestLeaderForwarder_Forward_UpstreamError_502(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	// Start a TLS server then immediately close it so the proxy gets a connection error
	leaderServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	port := leaderServer.Listener.Addr().(*net.TCPAddr).Port
	leaderServer.Close()

	lf := NewLeaderForwarder(port, 10)
	lf.SetLeaderIP("127.0.0.1")

	// Wrap in a TelemetryHandler so SetSpanError can propagate to the parent span
	th := &TelemetryHandler{
		handlerName: "testHandler",
		handler: func(w http.ResponseWriter, r *http.Request) {
			lf.Forward(w, r)
		},
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	th.handle(rw, req)

	assert.Equal(t, http.StatusBadGateway, rw.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 2) // forwarder span + parent telemetry span

	// Find the parent span (the telemetry handler span)
	var parentSpan mocktracer.Span
	for _, s := range spans {
		if s.OperationName() == "cluster_agent.api.request" {
			parentSpan = s
			break
		}
	}
	require.NotNil(t, parentSpan, "parent telemetry span should exist")
	// SetSpanError should have propagated the upstream error to the parent span
	assert.NotNil(t, parentSpan.Tag("error"), "upstream error should propagate to parent span")
}

func TestLeaderForwarder_Forward_WithLeader_Span(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	leaderServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer leaderServer.Close()

	port := leaderServer.Listener.Addr().(*net.TCPAddr).Port
	lf := NewLeaderForwarder(port, 10)
	lf.SetLeaderIP("127.0.0.1")

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	lf.Forward(rw, req)

	assert.Equal(t, http.StatusOK, rw.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.leader_forwarder.forward", span.OperationName())
	assert.Equal(t, false, span.Tag("forward.loop_detected"))
	assert.Equal(t, true, span.Tag("forward.proxy_available"))
}
