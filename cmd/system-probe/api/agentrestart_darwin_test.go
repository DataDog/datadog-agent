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

// syncAfterFunc replaces the timer so the callback runs synchronously inside
// the handler, before it returns.
func syncAfterFunc(_ time.Duration, f func()) *time.Timer {
	f()
	return nil
}

func TestHandleAgentRestart_Returns200Immediately(t *testing.T) {
	handler := newAgentRestartHandler(func(string) error { return nil }, syncAfterFunc)

	req := httptest.NewRequest(http.MethodPost, "/agent-restart", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleAgentRestart_ServiceRestartSequence(t *testing.T) {
	// expectedServices defines the exact order in which launchd services must be restarted.
	// Agent must come before sysprobe because restarting sysprobe sends SIGTERM to this process.
	expectedServices := []string{
		"system/com.datadoghq.agent",
		"system/com.datadoghq.sysprobe",
	}

	var called []string
	handler := newAgentRestartHandler(func(svc string) error {
		called = append(called, svc)
		return nil
	}, syncAfterFunc)

	req := httptest.NewRequest(http.MethodPost, "/agent-restart", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, expectedServices, called)
}
