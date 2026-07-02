// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func withMockKickstart(t *testing.T, mock func(string) error) {
	t.Helper()
	orig := kickstart
	kickstart = mock
	t.Cleanup(func() { kickstart = orig })
}

// withSyncAfterFunc replaces the timer so the callback runs synchronously inside
// handleAgentRestart, before the function returns. This prevents the real kickstart
// from being restored by t.Cleanup before the timer fires.
func withSyncAfterFunc(t *testing.T) {
	t.Helper()
	orig := afterFunc
	afterFunc = func(_ time.Duration, f func()) *time.Timer { f(); return nil }
	t.Cleanup(func() { afterFunc = orig })
}

func TestHandleAgentRestart_Returns200Immediately(t *testing.T) {
	withSyncAfterFunc(t)
	withMockKickstart(t, func(string) error { return nil })

	req := httptest.NewRequest(http.MethodPost, "/agent-restart", nil)
	rr := httptest.NewRecorder()

	handleAgentRestart(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleAgentRestart_ServiceRestartSequence(t *testing.T) {
	// expectedServices defines the exact order in which launchd services must be restarted.
	// Agent must come before sysprobe because restarting sysprobe sends SIGTERM to this process.
	expectedServices := []string{
		"system/com.datadoghq.agent",
		"system/com.datadoghq.sysprobe",
	}

	withSyncAfterFunc(t)

	var called []string
	withMockKickstart(t, func(svc string) error {
		called = append(called, svc)
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/agent-restart", nil)
	rr := httptest.NewRecorder()

	handleAgentRestart(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, expectedServices, called)
}
