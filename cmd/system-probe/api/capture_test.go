// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

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
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestCaptureRoutesAuthenticateEveryOperation(t *testing.T) {
	manager := telemetrycapture.NewManager("system-probe", "producer-version", "producer-commit")
	t.Cleanup(manager.Close)
	ipc := ipcmock.New(t)
	mux := http.NewServeMux()
	require.NoError(t, setupCaptureHandlers(mux, module.FactoryDependencies{CaptureManager: manager, Ipc: ipc}))

	for _, operation := range []string{"capabilities", "status", "prepare", "activate", "heartbeat", "records", "stop", ""} {
		method := http.MethodPost
		if operation == "capabilities" || operation == "status" {
			method = http.MethodGet
		}
		for _, token := range []string{"", "Bearer incorrect-token"} {
			t.Run(operation+"/"+token, func(t *testing.T) {
				request := httptest.NewRequest(method, "/eudm-capture/"+operation, bytes.NewBufferString("{}"))
				request.Header.Set("Authorization", token)
				response := httptest.NewRecorder()
				mux.ServeHTTP(response, request)
				expected := http.StatusForbidden
				if token == "" {
					expected = http.StatusUnauthorized
				}
				require.Equal(t, expected, response.Code)
				require.NotContains(t, response.Body.String(), "producer-commit")
			})
		}
	}
	require.Empty(t, manager.Status().SessionID)
}

func TestCaptureRoutesReadinessProtocolAndAcknowledgements(t *testing.T) {
	manager := telemetrycapture.NewManager("system-probe", "producer-version", "producer-commit")
	t.Cleanup(manager.Close)
	ipc := ipcmock.New(t)
	mux := http.NewServeMux()
	require.NoError(t, setupCaptureHandlers(mux, module.FactoryDependencies{CaptureManager: manager, Ipc: ipc}))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	request := func(operation string, value any, expected int, result any) {
		t.Helper()
		var body []byte
		method := http.MethodGet
		if value != nil {
			method = http.MethodPost
			var err error
			body, err = json.Marshal(value)
			require.NoError(t, err)
		}
		req, err := http.NewRequest(method, server.URL+"/eudm-capture/"+operation, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+ipc.GetAuthToken())
		response, err := server.Client().Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, expected, response.StatusCode)
		if result != nil {
			require.NoError(t, json.NewDecoder(response.Body).Decode(result))
		} else {
			_, err = io.Copy(io.Discard, response.Body)
			require.NoError(t, err)
		}
	}

	control := telemetrycapture.Control{ProtocolVersion: telemetrycapture.ProtocolVersion, SessionID: "system-probe-session-123"}
	prepare := telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Connections}}
	var status telemetrycapture.Status
	request("capabilities", nil, http.StatusOK, &status)
	require.Equal(t, "system-probe", status.Producer.Role)
	require.Empty(t, status.Capabilities, "constructed modules must not advertise an unavailable sender")
	request("prepare", prepare, http.StatusConflict, nil)
	require.NoError(t, manager.Register(telemetrycapture.Capability{Stream: telemetrycapture.Connections, Cadence: 30 * time.Second, ConnectionOwner: "system-probe"}))
	for _, operation := range []string{"prepare", "activate", "heartbeat", "records", "stop"} {
		value := map[string]any{"protocol_version": 1, "session_id": control.SessionID}
		if operation == "prepare" {
			value["streams"] = []string{"connections"}
		}
		request(operation, value, http.StatusBadRequest, nil)
	}
	request("prepare", prepare, http.StatusOK, &status)
	require.Equal(t, telemetrycapture.Prepared, status.State)
	request("activate", control, http.StatusOK, &status)
	require.Equal(t, telemetrycapture.Active, status.State)
	require.True(t, manager.Enabled())
	request("heartbeat", control, http.StatusOK, &status)

	reservation := manager.Begin(telemetrycapture.Connections, time.Now(), 30*time.Second, 4096)
	require.NotNil(t, reservation)
	require.True(t, reservation.Commit(telemetrycapture.Payload{Chunks: []telemetrycapture.Chunk{{Body: []byte("complete test group")}}}))
	request("stop", control, http.StatusOK, &status)
	require.Equal(t, telemetrycapture.Stopping, status.State)
	require.False(t, manager.Enabled())
	var batch struct {
		Status  telemetrycapture.Status   `json:"status"`
		Records []telemetrycapture.Record `json:"records"`
	}
	read := telemetrycapture.ReadRequest{Control: control}
	request("records", read, http.StatusOK, &batch)
	require.Len(t, batch.Records, 1)
	require.Equal(t, uint64(1), batch.Records[0].Sequence)
	request("records", read, http.StatusOK, &batch)
	require.Len(t, batch.Records, 1, "repeated reads must return the same accepted record")
	require.Equal(t, uint64(1), batch.Records[0].Sequence)
	read.Cursor = 2
	request("records", read, http.StatusBadRequest, nil)
	read.Cursor = 1
	request("records", read, http.StatusOK, &batch)
	require.Empty(t, batch.Records)
	request("stop", control, http.StatusOK, &status)
	require.Equal(t, telemetrycapture.Stopped, status.State)
	require.Equal(t, status.FinalSequence, status.Acknowledged)

	manager.Close()
	prepare.SessionID = "closed-producer-session"
	request("prepare", prepare, http.StatusServiceUnavailable, nil)
}

func TestCaptureRoutesRequireDaemonDependencies(t *testing.T) {
	mux := http.NewServeMux()
	require.NoError(t, setupCaptureHandlers(mux, module.FactoryDependencies{}))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/eudm-capture/status", nil))
	require.Equal(t, http.StatusNotFound, response.Code)
	manager := telemetrycapture.NewManager("system-probe", "test", "test")
	t.Cleanup(manager.Close)
	require.ErrorIs(t, setupCaptureHandlers(http.NewServeMux(), module.FactoryDependencies{CaptureManager: manager}), telemetrycapture.ErrClosed)
}
