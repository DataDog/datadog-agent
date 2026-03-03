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
)

func TestLeaderForwarder_SetLeaderIP(t *testing.T) {
	lf := NewLeaderForwarder(5005, 10)

	// Initially no leader IP
	assert.Equal(t, "", lf.GetLeaderIP())
	assert.Nil(t, lf.proxy)

	// Set leader IP
	lf.SetLeaderIP("1.1.1.1")
	assert.Equal(t, "1.1.1.1", lf.GetLeaderIP())
	assert.NotNil(t, lf.proxy)

	// Update leader IP
	lf.SetLeaderIP("2.2.2.2")
	assert.Equal(t, "2.2.2.2", lf.GetLeaderIP())
	assert.NotNil(t, lf.proxy)

	// Clear proxy with empty string - note: leaderIP is NOT cleared (returns early)
	lf.SetLeaderIP("")
	assert.Equal(t, "2.2.2.2", lf.GetLeaderIP()) // leaderIP unchanged
	assert.Nil(t, lf.proxy)                      // but proxy is cleared
}

func TestLeaderForwarder_Forward_NilProxy(t *testing.T) {
	lf := NewLeaderForwarder(5005, 10)

	// No leader set, proxy is nil
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	lf.Forward(rw, req)

	assert.Equal(t, http.StatusServiceUnavailable, rw.Code)
	assert.Equal(t, "true", rw.Header().Get("X-DCA-Forwarded"))
}

func TestLeaderForwarder_Forward_LoopDetection(t *testing.T) {
	// Track if leader server was called
	leaderCalled := false
	leaderServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		leaderCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer leaderServer.Close()

	port := leaderServer.Listener.Addr().(*net.TCPAddr).Port
	lf := NewLeaderForwarder(port, 10)
	lf.SetLeaderIP("127.0.0.1")

	// Request already has forward header (loop detection)
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	req.Header.Set("X-DCA-Follower-Forwarded", "true")

	lf.Forward(rw, req)

	// Loop detection should return 508 and NOT forward to leader
	assert.Equal(t, http.StatusLoopDetected, rw.Code)
	assert.Equal(t, "true", rw.Header().Get("X-DCA-Forwarded"))
	assert.False(t, leaderCalled, "Request should not be forwarded to leader when loop is detected")
}

func TestLeaderForwarder_Forward_WithLeader(t *testing.T) {
	// Create a test server to act as the leader
	leaderServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the forward header was added
		assert.Equal(t, "true", r.Header.Get("X-DCA-Follower-Forwarded"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("leader response"))
	}))
	defer leaderServer.Close()

	// Extract port from test server
	port := leaderServer.Listener.Addr().(*net.TCPAddr).Port
	lf := NewLeaderForwarder(port, 10)
	lf.SetLeaderIP("127.0.0.1")

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)

	lf.Forward(rw, req)

	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Equal(t, "true", rw.Header().Get("X-DCA-Forwarded"))
	assert.Equal(t, "leader response", rw.Body.String())
}
