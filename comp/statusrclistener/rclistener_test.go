// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package statusrclistener

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	pkgconfigmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// mockStatusComponent is a minimal implementation of status.Component for tests.
type mockStatusComponent struct {
	statusJSON []byte
	err        error
}

func (m *mockStatusComponent) GetStatus(_ string, _ bool, _ ...string) ([]byte, error) {
	return m.statusJSON, m.err
}
func (m *mockStatusComponent) GetStatusBySections(_ []string, _ string, _ bool) ([]byte, error) {
	return m.statusJSON, m.err
}
func (m *mockStatusComponent) GetSections() []string { return nil }

func makeTask(t *testing.T, taskJSON string) rcclienttypes.AgentTaskConfig {
	t.Helper()
	task, err := rcclienttypes.ParseConfigAgentTask([]byte(taskJSON), state.Metadata{})
	require.NoError(t, err)
	return task
}

func TestOnAgentTaskEvent_WrongTaskType(t *testing.T) {
	l := &statusRCListener{
		status: &mockStatusComponent{},
		config: pkgconfigmock.New(t),
	}

	task := makeTask(t, `{"task_type":"flare","uuid":"u1","args":{"collect_id":"cid"}}`)
	processed, err := l.onAgentTaskEvent(rcclienttypes.TaskFlare, task)

	assert.False(t, processed)
	assert.NoError(t, err)
}

func TestOnAgentTaskEvent_MissingCollectID(t *testing.T) {
	l := &statusRCListener{
		status: &mockStatusComponent{},
		config: pkgconfigmock.New(t),
	}

	task := makeTask(t, `{"task_type":"status","uuid":"u1","args":{}}`)
	processed, err := l.onAgentTaskEvent(rcclienttypes.TaskStatus, task)

	assert.True(t, processed)
	assert.ErrorContains(t, err, "collect_id was not provided")
}

func TestOnAgentTaskEvent_GetStatusError(t *testing.T) {
	l := &statusRCListener{
		status: &mockStatusComponent{err: errors.New("collector not ready")},
		config: pkgconfigmock.New(t),
	}

	task := makeTask(t, `{"task_type":"status","uuid":"u1","args":{"collect_id":"cid-1"}}`)
	processed, err := l.onAgentTaskEvent(rcclienttypes.TaskStatus, task)

	assert.True(t, processed)
	assert.ErrorContains(t, err, "failed to get agent status")
	assert.ErrorContains(t, err, "collector not ready")
}

func TestOnAgentTaskEvent_Success(t *testing.T) {
	fakeStatus := map[string]interface{}{"version": "7.99.0", "status": "ok"}
	fakeStatusJSON, err := json.Marshal(fakeStatus)
	require.NoError(t, err)

	var receivedPayload statusPayload
	var receivedAPIKey string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fleetStatusPath, r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		receivedAPIKey = r.Header.Get("DD-API-KEY")

		require.NoError(t, json.NewDecoder(r.Body).Decode(&receivedPayload))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := pkgconfigmock.New(t)
	cfg.SetWithoutSource("dd_url", srv.URL)
	cfg.SetWithoutSource("api_key", "test-api-key")

	l := &statusRCListener{
		status: &mockStatusComponent{statusJSON: fakeStatusJSON},
		config: cfg,
	}

	task := makeTask(t, `{"task_type":"status","uuid":"u1","args":{"collect_id":"cid-42"}}`)
	processed, err := l.onAgentTaskEvent(rcclienttypes.TaskStatus, task)

	require.NoError(t, err)
	assert.True(t, processed)
	assert.Equal(t, "test-api-key", receivedAPIKey)
	assert.Equal(t, "cid-42", receivedPayload.CollectID)

	var gotStatus map[string]interface{}
	require.NoError(t, json.Unmarshal(receivedPayload.StatusContent, &gotStatus))
	assert.Equal(t, "7.99.0", gotStatus["version"])
}

func TestOnAgentTaskEvent_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := pkgconfigmock.New(t)
	cfg.SetWithoutSource("dd_url", srv.URL)

	l := &statusRCListener{
		status: &mockStatusComponent{statusJSON: []byte(`{}`)},
		config: cfg,
	}

	task := makeTask(t, `{"task_type":"status","uuid":"u1","args":{"collect_id":"cid-99"}}`)
	processed, err := l.onAgentTaskEvent(rcclienttypes.TaskStatus, task)

	assert.True(t, processed)
	assert.ErrorContains(t, err, "fleet-api returned non-200 status 500")
}
