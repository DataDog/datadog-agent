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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/mocktracer"
)

func TestRejectOrForwardLeaderQuery_AsLeader_NoSpan(t *testing.T) {
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

	spans := mt.FinishedSpans()
	assert.Len(t, spans, 0, "No span should be created when node is the leader")
}

func TestRejectOrForwardLeaderQuery_AsFollower_Span(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: true,
		le:                    &mockLeaderEngine{isLeader: false, leaderIP: "2.2.2.2"},
		leaderForwarder:       &fakeLeaderForwarder{},
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	result := lph.rejectOrForwardLeaderQuery(rw, req)
	assert.True(t, result)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.leader_proxy.forward", span.OperationName())
	assert.Equal(t, true, span.Tag("forwarded"))
	assert.Equal(t, "2.2.2.2", span.Tag("forward.leader_ip"))
	assert.Nil(t, span.Tag("error"), "No error tag on success path")
}

func TestRejectOrForwardLeaderQuery_NoForwarder_Span(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: true,
		le:                    &mockLeaderEngine{isLeader: false, leaderIP: "1.1.1.1"},
		leaderForwarder:       nil,
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	result := lph.rejectOrForwardLeaderQuery(rw, req)
	assert.True(t, result)
	assert.Equal(t, http.StatusServiceUnavailable, rw.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.leader_proxy.forward", span.OperationName())
	assert.Equal(t, false, span.Tag("forwarded"))
	assert.Equal(t, "forwarder_unavailable", span.Tag("forward.failure_mode"))
	assert.NotNil(t, span.Tag("error"), "Error should be set on the span")
}

func TestRejectOrForwardLeaderQuery_GetLeaderIPError_Span(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: true,
		le:                    &mockLeaderEngine{isLeader: false, leaderIPErr: errors.New("connection refused")},
		leaderForwarder:       &fakeLeaderForwarder{},
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	result := lph.rejectOrForwardLeaderQuery(rw, req)
	assert.True(t, result)
	assert.Equal(t, http.StatusServiceUnavailable, rw.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.leader_proxy.forward", span.OperationName())
	assert.Equal(t, false, span.Tag("forwarded"))
	assert.Equal(t, "leader_ip_unavailable", span.Tag("forward.failure_mode"))
	assert.NotNil(t, span.Tag("error"), "Error should be set on the span")
}

func TestRejectOrForwardLeaderQuery_Disabled_NoSpan(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: false,
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	result := lph.rejectOrForwardLeaderQuery(rw, req)
	assert.False(t, result)

	spans := mt.FinishedSpans()
	assert.Len(t, spans, 0, "No span should be created when leader election is disabled")
}
