// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// stubGetSIDForPID replaces the getSIDForPID package var (the seam sidForPIDHandler
// calls through) for the duration of the test, restoring the original
// implementation on cleanup. Mirrors this repo's existing test-seam convention for
// package-level function vars, e.g. peeridentity_darwin.go's consoleDevicePath.
func stubGetSIDForPID(t *testing.T, fn func(int32) (string, error)) {
	orig := getSIDForPID
	getSIDForPID = fn
	t.Cleanup(func() { getSIDForPID = orig })
}

func TestSidForPIDHandler_Success(t *testing.T) {
	stubGetSIDForPID(t, func(pid int32) (string, error) {
		assert.EqualValues(t, 4242, pid)
		return "S-1-5-21-1-2-3-1001", nil
	})

	req := httptest.NewRequest(http.MethodGet, "/pid/4242/sid", nil)
	req.SetPathValue("pid", "4242")
	rr := httptest.NewRecorder()

	sidForPIDHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "S-1-5-21-1-2-3-1001", rr.Body.String())
	assert.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"))
}

func TestSidForPIDHandler_InvalidPidInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		pid  string
	}{
		{"non-numeric", "not-a-number"},
		{"empty", ""},
		{"zero", "0"},
		{"negative", "-5"},
		{"overflows int32", "99999999999"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubGetSIDForPID(t, func(pid int32) (string, error) {
				t.Fatalf("getSIDForPID should not be called for invalid pid %q, got pid=%d", tc.pid, pid)
				return "", nil
			})

			req := httptest.NewRequest(http.MethodGet, "/pid/"+tc.pid+"/sid", nil)
			req.SetPathValue("pid", tc.pid)
			rr := httptest.NewRecorder()

			sidForPIDHandler(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), "invalid pid")
		})
	}
}

func TestSidForPIDHandler_ErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "process not found classifies as 404 (ERROR_INVALID_PARAMETER)",
			err:        fmt.Errorf("failed to open process 4242: %w", windows.ERROR_INVALID_PARAMETER),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "access denied classifies as 403 (ERROR_ACCESS_DENIED)",
			err:        fmt.Errorf("failed to open process token for pid 4242: %w", windows.ERROR_ACCESS_DENIED),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "unclassified error falls back to 500",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubGetSIDForPID(t, func(int32) (string, error) {
				return "", tc.err
			})

			req := httptest.NewRequest(http.MethodGet, "/pid/4242/sid", nil)
			req.SetPathValue("pid", "4242")
			rr := httptest.NewRecorder()

			sidForPIDHandler(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.err.Error())
		})
	}
}

// TestSidForPIDHandler_ThroughServeMux is a smoke test that registerPlatformHandlers
// (server_windows.go) actually wires "GET /pid/{pid}/sid" to sidForPIDHandler on a
// real *http.ServeMux, and that the {pid} path segment ServeMux extracts is what
// reaches the handler - guarding against a future accidental removal or typo in
// that one-line registration.
func TestSidForPIDHandler_ThroughServeMux(t *testing.T) {
	stubGetSIDForPID(t, func(pid int32) (string, error) {
		assert.EqualValues(t, 4242, pid)
		return "S-1-5-21-1-2-3-1001", nil
	})

	mux := http.NewServeMux()
	registerPlatformHandlers(APIServerDeps{}, mux)

	req := httptest.NewRequest(http.MethodGet, "/pid/4242/sid", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "S-1-5-21-1-2-3-1001", rr.Body.String())
}
