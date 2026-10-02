// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package apiserverimpl

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestCaptureRoutesRequireProcessIPCAuthentication(t *testing.T) {
	m := telemetrycapture.NewManager("process-agent", "fixture", "fixture")
	defer m.Close()
	ipc := ipcmock.New(t)
	mux := http.NewServeMux()
	require.NoError(t, mountCaptureHandlers(mux, m, ipc.HTTPMiddleware))
	for _, operation := range []struct{ method, path string }{
		{http.MethodGet, "capabilities"}, {http.MethodGet, "status"},
		{http.MethodPost, "prepare"}, {http.MethodPost, "activate"},
		{http.MethodPost, "heartbeat"}, {http.MethodPost, "records"}, {http.MethodPost, "stop"},
	} {
		for _, token := range []string{"", "Bearer wrong-token"} {
			req := httptest.NewRequest(operation.method, "/eudm-capture/"+operation.path, bytes.NewBufferString(`{}`))
			req.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, req)
			if token == "" {
				require.Equal(t, http.StatusUnauthorized, response.Code, operation.path)
			} else {
				require.Equal(t, http.StatusForbidden, response.Code, operation.path)
			}
		}
	}
	require.Empty(t, m.Status().SessionID)
}

func TestCaptureRoutesValidateProtocolDrainAndShutdown(t *testing.T) {
	m := telemetrycapture.NewManager("process-agent", "fixture", "fixture")
	defer m.Close()
	ipc := ipcmock.New(t)
	mux := http.NewServeMux()
	require.NoError(t, mountCaptureHandlers(mux, m, ipc.HTTPMiddleware))
	server := httptest.NewServer(ipc.HTTPMiddleware(mux))
	defer server.Close()
	request := func(method, operation, body string, expected int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+"/eudm-capture/"+operation, bytes.NewBufferString(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+ipc.GetAuthToken())
		response, err := server.Client().Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		result, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, expected, response.StatusCode, string(result))
		return result
	}
	var initial telemetrycapture.Status
	require.NoError(t, json.Unmarshal(request(http.MethodGet, "capabilities", "", http.StatusOK), &initial))
	require.Empty(t, initial.Capabilities)
	control := `{"protocol_version":1,"session_id":"process-api-session"}`
	prepare := `{"protocol_version":1,"session_id":"process-api-session","streams":["processes"]}`
	request(http.MethodPost, "prepare", prepare, http.StatusConflict)
	require.NoError(t, m.Register(telemetrycapture.Capability{Stream: telemetrycapture.Processes, Cadence: time.Second}))
	for _, body := range []string{`{`, `{"protocol_version":2,"session_id":"process-api-session","streams":["processes"]}`, `{"protocol_version":1,"session_id":"process-api-session","streams":["processes"],"unexpected":"fixture-sensitive"}`} {
		result := request(http.MethodPost, "prepare", body, http.StatusBadRequest)
		require.NotContains(t, string(result), "fixture-sensitive")
	}
	request(http.MethodPost, "prepare", prepare, http.StatusOK)
	request(http.MethodPost, "activate", control, http.StatusOK)
	request(http.MethodPost, "heartbeat", control, http.StatusOK)
	reservation := m.Begin(telemetrycapture.Processes, time.Now(), time.Second, 1024)
	require.NotNil(t, reservation)
	require.True(t, reservation.Commit(telemetrycapture.Payload{Chunks: []telemetrycapture.Chunk{{Body: []byte("fixture-chunk")}}}))
	var batch struct {
		Status  telemetrycapture.Status    `json:"status"`
		Records []*telemetrycapture.Record `json:"records"`
	}
	require.NoError(t, json.Unmarshal(request(http.MethodPost, "records", control, http.StatusOK), &batch))
	require.Len(t, batch.Records, 1)
	require.Equal(t, uint64(1), batch.Records[0].Sequence)
	request(http.MethodPost, "records", `{"protocol_version":1,"session_id":"process-api-session","cursor":2}`, http.StatusBadRequest)
	request(http.MethodPost, "stop", control, http.StatusOK)
	require.Equal(t, telemetrycapture.Stopping, m.Status().State)
	request(http.MethodPost, "records", `{"protocol_version":1,"session_id":"process-api-session","cursor":1}`, http.StatusOK)
	request(http.MethodPost, "stop", control, http.StatusOK)
	require.Equal(t, telemetrycapture.Stopped, m.Status().State)
	m.Close()
	request(http.MethodPost, "prepare", prepare, http.StatusServiceUnavailable)
}

func TestCaptureRoutesAbsentWithoutDaemonManager(t *testing.T) {
	mux := http.NewServeMux()
	require.NoError(t, mountCaptureHandlers(mux, nil, nil))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/eudm-capture/status", nil))
	require.Equal(t, http.StatusNotFound, response.Code)
}
