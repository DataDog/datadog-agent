// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

const validOwnerSIDQuery = "/connection/owner-sid?family=4&laddr=127.0.0.1&lport=54321&raddr=127.0.0.1&rport=5002"

// stubGetSIDForConnectionOwner replaces the getSIDForConnectionOwner package var (the seam
// connectionOwnerSIDHandler calls through) for the duration of the test, restoring the original
// implementation on cleanup. Mirrors this repo's existing test-seam convention for package-level function
// vars, e.g. peeridentity_darwin.go's consoleDevicePath.
func stubGetSIDForConnectionOwner(t *testing.T, fn func(uint32, net.IP, int, net.IP, int) (string, error)) {
	orig := getSIDForConnectionOwner
	getSIDForConnectionOwner = fn
	t.Cleanup(func() { getSIDForConnectionOwner = orig })
}

func TestConnectionOwnerSIDHandler_Success(t *testing.T) {
	stubGetSIDForConnectionOwner(t, func(family uint32, localAddr net.IP, localPort int, remoteAddr net.IP, remotePort int) (string, error) {
		assert.EqualValues(t, windows.AF_INET, family)
		assert.True(t, net.ParseIP("127.0.0.1").Equal(localAddr))
		assert.Equal(t, 54321, localPort)
		assert.True(t, net.ParseIP("127.0.0.1").Equal(remoteAddr))
		assert.Equal(t, 5002, remotePort)
		return "S-1-5-21-1-2-3-1001", nil
	})

	req := httptest.NewRequest(http.MethodGet, validOwnerSIDQuery, nil)
	rr := httptest.NewRecorder()

	connectionOwnerSIDHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "S-1-5-21-1-2-3-1001", rr.Body.String())
	assert.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"))
}

func TestConnectionOwnerSIDHandler_IPv6FamilyMapping(t *testing.T) {
	stubGetSIDForConnectionOwner(t, func(family uint32, _ net.IP, _ int, _ net.IP, _ int) (string, error) {
		assert.EqualValues(t, windows.AF_INET6, family)
		return "S-1-5-21-1-2-3-1001", nil
	})

	req := httptest.NewRequest(http.MethodGet, "/connection/owner-sid?family=6&laddr=::1&lport=54321&raddr=::1&rport=5002", nil)
	rr := httptest.NewRecorder()

	connectionOwnerSIDHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

func TestConnectionOwnerSIDHandler_InvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
	}{
		{"missing family", "/connection/owner-sid?laddr=127.0.0.1&lport=54321&raddr=127.0.0.1&rport=5002"},
		{"bad family", "/connection/owner-sid?family=9&laddr=127.0.0.1&lport=54321&raddr=127.0.0.1&rport=5002"},
		{"missing laddr", "/connection/owner-sid?family=4&lport=54321&raddr=127.0.0.1&rport=5002"},
		{"unparseable raddr", "/connection/owner-sid?family=4&laddr=127.0.0.1&lport=54321&raddr=nope&rport=5002"},
		{"missing lport", "/connection/owner-sid?family=4&laddr=127.0.0.1&raddr=127.0.0.1&rport=5002"},
		{"non-numeric rport", "/connection/owner-sid?family=4&laddr=127.0.0.1&lport=54321&raddr=127.0.0.1&rport=x"},
		{"zero lport", "/connection/owner-sid?family=4&laddr=127.0.0.1&lport=0&raddr=127.0.0.1&rport=5002"},
		{"out-of-range rport", "/connection/owner-sid?family=4&laddr=127.0.0.1&lport=54321&raddr=127.0.0.1&rport=70000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubGetSIDForConnectionOwner(t, func(uint32, net.IP, int, net.IP, int) (string, error) {
				t.Fatalf("getSIDForConnectionOwner should not be called for invalid input %q", tc.query)
				return "", nil
			})

			req := httptest.NewRequest(http.MethodGet, tc.query, nil)
			rr := httptest.NewRecorder()

			connectionOwnerSIDHandler(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func TestConnectionOwnerSIDHandler_ErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "no owner classifies as 404",
			err:        fmt.Errorf("wrapped: %w", procutil.ErrConnectionOwnerNotFound),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "owner changed classifies as 409",
			err:        fmt.Errorf("wrapped: %w", procutil.ErrConnectionOwnerChanged),
			wantStatus: http.StatusConflict,
		},
		{
			name:       "access denied classifies as 403 (ERROR_ACCESS_DENIED)",
			err:        fmt.Errorf("failed to open process token: %w", windows.ERROR_ACCESS_DENIED),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "unclassified error falls back to 500",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubGetSIDForConnectionOwner(t, func(uint32, net.IP, int, net.IP, int) (string, error) {
				return "", tc.err
			})

			req := httptest.NewRequest(http.MethodGet, validOwnerSIDQuery, nil)
			rr := httptest.NewRecorder()

			connectionOwnerSIDHandler(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.err.Error())
		})
	}
}

// TestConnectionOwnerSIDHandler_ThroughServeMux is a smoke test that registerPlatformHandlers
// (server_windows.go) actually wires "GET /connection/owner-sid" to connectionOwnerSIDHandler on a real
// *http.ServeMux, guarding against a future accidental removal or typo in that one-line registration.
func TestConnectionOwnerSIDHandler_ThroughServeMux(t *testing.T) {
	stubGetSIDForConnectionOwner(t, func(uint32, net.IP, int, net.IP, int) (string, error) {
		return "S-1-5-21-1-2-3-1001", nil
	})

	mux := http.NewServeMux()
	registerPlatformHandlers(APIServerDeps{}, mux)

	req := httptest.NewRequest(http.MethodGet, validOwnerSIDQuery, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "S-1-5-21-1-2-3-1001", rr.Body.String())
}
