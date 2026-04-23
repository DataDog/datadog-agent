// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver && test

package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectOrForwardLeaderQuery_AsLeader_NoError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: true,
		le:                    &mockLeaderEngine{isLeader: true, leaderIP: "1.1.1.1"},
		leaderForwarder:       &fakeLeaderForwarder{},
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	result := lph.rejectOrForwardLeaderQuery(rw, req)
	assert.False(t, result)
}

func TestRejectOrForwardLeaderQuery_AsFollower_NoError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: true,
		le:                    &mockLeaderEngine{isLeader: false, leaderIP: "2.2.2.2"},
		leaderForwarder:       &fakeLeaderForwarder{},
	}

	th := &TelemetryHandler{
		handlerName: "testHandler",
		handler: func(w http.ResponseWriter, r *http.Request) {
			lph.rejectOrForwardLeaderQuery(w, r)
		},
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	th.handle(rw, req)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.api.request", span.OperationName())
	assert.Nil(t, span.Tag("error.message"), "No error on successful forward")
}

func TestRejectOrForwardLeaderQuery_NoForwarder_PropagatesError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: true,
		le:                    &mockLeaderEngine{isLeader: false, leaderIP: "1.1.1.1"},
		leaderForwarder:       nil,
	}

	th := &TelemetryHandler{
		handlerName: "testHandler",
		handler: func(w http.ResponseWriter, r *http.Request) {
			lph.rejectOrForwardLeaderQuery(w, r)
		},
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	th.handle(rw, req)

	assert.Equal(t, http.StatusServiceUnavailable, rw.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.api.request", span.OperationName())
	assert.NotNil(t, span.Tag("error.message"), "forwarder unavailable error should propagate to parent span")
}

func TestRejectOrForwardLeaderQuery_GetLeaderIPError_PropagatesError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: true,
		le:                    &mockLeaderEngine{isLeader: false, leaderIPErr: errors.New("connection refused")},
		leaderForwarder:       &fakeLeaderForwarder{},
	}

	th := &TelemetryHandler{
		handlerName: "testHandler",
		handler: func(w http.ResponseWriter, r *http.Request) {
			lph.rejectOrForwardLeaderQuery(w, r)
		},
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	th.handle(rw, req)

	assert.Equal(t, http.StatusServiceUnavailable, rw.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.api.request", span.OperationName())
	assert.NotNil(t, span.Tag("error.message"), "leader IP error should propagate to parent span")
}

func TestRejectOrForwardLeaderQuery_Disabled(t *testing.T) {
	lph := &LeaderProxyHandler{
		leaderElectionEnabled: false,
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	result := lph.rejectOrForwardLeaderQuery(rw, req)
	assert.False(t, result)
}
