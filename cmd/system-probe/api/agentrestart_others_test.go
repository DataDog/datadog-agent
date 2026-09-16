// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleAgentRestart_NotSupportedOnNonDarwin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/agent-restart", nil)
	rr := httptest.NewRecorder()

	handleAgentRestart(rr, req)

	assert.Equal(t, http.StatusNotImplemented, rr.Code)
	assert.Contains(t, rr.Body.String(), "not supported on this platform")
}
