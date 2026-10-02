// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package apiimpl

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/observability"
	collector "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/stretchr/testify/require"
)

type captureScheduleCollector struct {
	collector.Component
	checks []check.Info
}

func (c *captureScheduleCollector) MapOverChecks(callback func([]check.Info)) { callback(c.checks) }

type captureScheduledCheck struct {
	check.Info
	name    string
	cadence time.Duration
}

func (c captureScheduledCheck) String() string          { return c.name }
func (c captureScheduledCheck) Interval() time.Duration { return c.cadence }

func TestCaptureSchedulesFollowScheduledChecksWithoutCollecting(t *testing.T) {
	ipc := ipcmock.New(t)
	manager := telemetrycapture.NewManager("core-agent", "fixture", "fixture")
	defer manager.Close()
	require.NoError(t, manager.Register(telemetrycapture.Capability{Stream: telemetrycapture.Metrics, Cadence: 15 * time.Second}))
	checks := &captureScheduleCollector{checks: []check.Info{
		captureScheduledCheck{name: "cpu", cadence: 15 * time.Second},
		captureScheduledCheck{name: "battery", cadence: 5 * time.Minute},
		captureScheduledCheck{name: "battery", cadence: 7 * time.Minute},
		captureScheduledCheck{name: "network", cadence: 15 * time.Second},
		captureScheduledCheck{name: "wlan", cadence: 0},
		captureScheduledCheck{name: "private-integration-name", cadence: time.Hour},
	}}
	server := &apiServer{ipc: ipc, captureManager: manager, collector: checks}
	mux := http.NewServeMux()
	require.NoError(t, server.mountCapture(mux))
	read := func() telemetrycapture.Status {
		r := httptest.NewRequest(http.MethodGet, "/agent/eudm-capture/capabilities", nil)
		r.Header.Set("Authorization", "Bearer "+ipc.GetAuthToken())
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		require.Equal(t, http.StatusOK, w.Code)
		require.NotContains(t, w.Body.String(), "private-integration-name")
		var status telemetrycapture.Status
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
		return status
	}
	require.Equal(t, []telemetrycapture.MetricSchedule{
		{Family: "battery", Cadence: 7 * time.Minute},
		{Family: "cpu", Cadence: 15 * time.Second},
		{Family: "network", Cadence: 15 * time.Second},
	}, read().Capabilities[0].MetricSchedules)
	// Battery-less or disabled checks are absent from the actual scheduled list;
	// the API must not manufacture a battery schedule from a default interval.
	checks.checks = checks.checks[:1]
	require.Equal(t, []telemetrycapture.MetricSchedule{{Family: "cpu", Cadence: 15 * time.Second}}, read().Capabilities[0].MetricSchedules)
	checks.checks = nil
	require.Empty(t, read().Capabilities[0].MetricSchedules)
	server.collector = nil
	require.Empty(t, read().Capabilities[0].MetricSchedules)
}

func TestCaptureRoutesAuthenticateAndDrain(t *testing.T) {
	ipc := ipcmock.New(t)
	manager := telemetrycapture.NewManager("core-agent", "fixture", "fixture")
	defer manager.Close()
	require.NoError(t, manager.Register(telemetrycapture.Capability{Stream: telemetrycapture.Software, Cadence: time.Minute}))
	server := &apiServer{ipc: ipc, captureManager: manager}
	mux := http.NewServeMux()
	require.NoError(t, server.mountCapture(mux))
	// Exercise the real response-writer wrappers: bounded reads require deadline
	// support to reach the underlying connection through observability middleware.
	ts := httptest.NewServer(observability.LogResponseHandler("capture fixture")(mux))
	defer ts.Close()
	request := func(method, path string, body any, authenticated bool) (int, []byte) {
		t.Helper()
		var data []byte
		if body != nil {
			var err error
			data, err = json.Marshal(body)
			require.NoError(t, err)
		}
		req, err := http.NewRequest(method, ts.URL+"/agent/eudm-capture/"+path, bytes.NewReader(data))
		require.NoError(t, err)
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+ipc.GetAuthToken())
		}
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		result, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, result
	}
	for _, path := range []string{"status", "capabilities", "prepare", "activate", "heartbeat", "records", "stop"} {
		method := "POST"
		if path == "status" || path == "capabilities" {
			method = "GET"
		}
		code, _ := request(method, path, nil, false)
		require.Equal(t, http.StatusUnauthorized, code)
	}
	for _, path := range []string{"prepare", "activate", "heartbeat", "records", "stop"} {
		body := map[string]any{"protocol_version": 99, "session_id": "capture-core-api-session"}
		if path == "prepare" {
			body["streams"] = []string{"software"}
		}
		code, data := request("POST", path, body, true)
		require.Equal(t, http.StatusBadRequest, code)
		require.NotContains(t, string(data), "capture-core-api-session")
	}
	control := telemetrycapture.Control{ProtocolVersion: 1, SessionID: "capture-core-api-session"}
	code, _ := request("POST", "prepare", telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Software}}, true)
	require.Equal(t, http.StatusOK, code)
	code, _ = request("POST", "activate", control, true)
	require.Equal(t, http.StatusOK, code)
	manager.Observe(telemetrycapture.Software, time.Now(), time.Minute, 1024, func() telemetrycapture.Payload {
		return telemetrycapture.Payload{Software: &telemetrycapture.Message{Body: []byte("owned-record"), Timestamp: 1}}
	})
	code, statusBody := request("GET", "status", nil, true)
	require.Equal(t, http.StatusOK, code)
	require.NotContains(t, string(statusBody), "owned-record")
	code, body := request("POST", "records", telemetrycapture.ReadRequest{Control: control}, true)
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, string(body), "records")
	var batch struct {
		Records []telemetrycapture.Record `json:"records"`
	}
	require.NoError(t, json.Unmarshal(body, &batch))
	require.Len(t, batch.Records, 1)
	code, _ = request("POST", "records", telemetrycapture.ReadRequest{Control: control, Cursor: batch.Records[0].Sequence}, true)
	require.Equal(t, http.StatusOK, code)
	code, body = request("POST", "stop", control, true)
	require.Equal(t, http.StatusOK, code)
	var status telemetrycapture.Status
	require.NoError(t, json.Unmarshal(body, &status))
	require.Equal(t, telemetrycapture.Stopped, status.State)
	require.Equal(t, status.FinalSequence, status.Acknowledged)
	manager.Close()
	code, _ = request("POST", "prepare", telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Software}}, true)
	require.Equal(t, http.StatusServiceUnavailable, code)
}
