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

// fakeLeaderForwarder is a fake implementation of the forwarder for testing purposes.
// It tracks leader IP changes and forward calls for verifying leadership transition behavior.
type fakeLeaderForwarder struct {
	currentLeaderIP     string
	leaderIPChangeCount int
	forwardCallCount    int
}

func (f *fakeLeaderForwarder) SetLeaderIP(ip string) {
	f.currentLeaderIP = ip
	f.leaderIPChangeCount++
}

func (f *fakeLeaderForwarder) GetLeaderIP() string {
	return f.currentLeaderIP
}

func (f *fakeLeaderForwarder) Forward(w http.ResponseWriter, _ *http.Request) {
	f.forwardCallCount++
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

// TestRejectOrForwardLeaderQuery_LeadershipTransition tests the behavior when
// leadership changes between requests (leader to follower and back).
func TestRejectOrForwardLeaderQuery_LeadershipTransition(t *testing.T) {
	mockEngine := &mockLeaderEngine{
		isLeader: true,
		leaderIP: "1.1.1.1",
	}
	forwarder := &fakeLeaderForwarder{}

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: true,
		le:                    mockEngine,
		leaderForwarder:       forwarder,
	}

	// First request: we are the leader, should handle locally
	rw1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("GET", "http://example.com/foo", nil)
	assert.False(t, lph.rejectOrForwardLeaderQuery(rw1, req1), "Should handle locally as leader")
	assert.Equal(t, 0, forwarder.forwardCallCount, "Should not forward when leader")

	// Simulate leadership loss
	mockEngine.isLeader = false
	mockEngine.leaderIP = "2.2.2.2"

	// Second request: we lost leadership, should forward to new leader
	rw2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "http://example.com/foo", nil)
	assert.True(t, lph.rejectOrForwardLeaderQuery(rw2, req2), "Should forward as follower")
	assert.Equal(t, 1, forwarder.forwardCallCount, "Should forward once")
	assert.Equal(t, "2.2.2.2", forwarder.currentLeaderIP, "Should update to new leader IP")

	// Simulate regaining leadership
	mockEngine.isLeader = true

	// Third request: we became the leader again, should handle locally
	rw3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("GET", "http://example.com/foo", nil)
	assert.False(t, lph.rejectOrForwardLeaderQuery(rw3, req3), "Should handle locally as new leader")
	assert.Equal(t, 1, forwarder.forwardCallCount, "Should not forward additional requests")
}

// TestRejectOrForwardLeaderQuery_LeaderIPChange tests that the forwarder is updated
// when the leader IP changes while we remain a follower.
func TestRejectOrForwardLeaderQuery_LeaderIPChange(t *testing.T) {
	mockEngine := &mockLeaderEngine{
		isLeader: false,
		leaderIP: "1.1.1.1",
	}
	forwarder := &fakeLeaderForwarder{
		currentLeaderIP: "1.1.1.1", // Already knows old leader
	}

	lph := &LeaderProxyHandler{
		leaderElectionEnabled: true,
		le:                    mockEngine,
		leaderForwarder:       forwarder,
	}

	// First request: forward to current leader
	rw1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("GET", "http://example.com/foo", nil)
	assert.True(t, lph.rejectOrForwardLeaderQuery(rw1, req1))
	assert.Equal(t, 1, forwarder.forwardCallCount)
	// IP didn't change, so SetLeaderIP should not have been called
	assert.Equal(t, 0, forwarder.leaderIPChangeCount, "Should not update IP when unchanged")

	// Simulate leader failover - new leader elected
	mockEngine.leaderIP = "2.2.2.2"

	// Second request: should detect IP change and update forwarder
	rw2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "http://example.com/foo", nil)
	assert.True(t, lph.rejectOrForwardLeaderQuery(rw2, req2))
	assert.Equal(t, 2, forwarder.forwardCallCount)
	assert.Equal(t, 1, forwarder.leaderIPChangeCount, "Should update IP once")
	assert.Equal(t, "2.2.2.2", forwarder.currentLeaderIP, "Should have new leader IP")

	// Third request: IP hasn't changed again
	rw3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("GET", "http://example.com/foo", nil)
	assert.True(t, lph.rejectOrForwardLeaderQuery(rw3, req3))
	assert.Equal(t, 3, forwarder.forwardCallCount)
	assert.Equal(t, 1, forwarder.leaderIPChangeCount, "Should not update IP when unchanged")
}
