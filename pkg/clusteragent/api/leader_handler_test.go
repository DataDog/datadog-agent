// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver && test

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockLeaderEngine is our mock implementation of the leaderEngine interface
type mockLeaderEngine struct {
	isLeader bool
	leaderIP string
}

func (m *mockLeaderEngine) IsLeader() bool {
	return m.isLeader
}

func (m *mockLeaderEngine) GetLeaderIP() (string, error) {
	return m.leaderIP, nil
}

// fakeLeaderForwarder is a fake implementation of the forwarder for testing purposes
type fakeLeaderForwarder struct{}

// SetLeaderIP does nothing
func (f *fakeLeaderForwarder) SetLeaderIP(_ string) {}

// Forward returns ok
func (f *fakeLeaderForwarder) Forward(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestRejectOrForwardLeaderQuery_LeaderElectionDisabled(t *testing.T) {
	lph := &LeaderProxyHandler{
		leaderElectionEnabled: false,
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	assert.False(t, lph.rejectOrForwardLeaderQuery(rw, req))
}

func TestRejectOrForwardLeaderQuery_AsFollower(t *testing.T) {
	mockEngine := &mockLeaderEngine{
		isLeader: false,
		leaderIP: "1.1.1.1",
	}

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: true,
		le:                    mockEngine,
		leaderForwarder:       &fakeLeaderForwarder{},
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	assert.True(t, lph.rejectOrForwardLeaderQuery(rw, req))
	assert.Equal(t, http.StatusOK, rw.Code)
}

func TestRejectOrForwardLeaderQuery_AsLeader(t *testing.T) {
	mockEngine := &mockLeaderEngine{
		isLeader: true,
		leaderIP: "1.1.1.1",
	}

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: true,
		le:                    mockEngine,
		leaderForwarder:       &fakeLeaderForwarder{},
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	assert.False(t, lph.rejectOrForwardLeaderQuery(rw, req))
}
