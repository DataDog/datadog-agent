// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/characterization"
)

func TestLoadCharacterizationLifecycle(t *testing.T) {
	manager := characterization.NewManager()
	handler := loadCharacterizationHandler(manager, true)

	start := httptest.NewRecorder()
	handler(start, httptest.NewRequest(http.MethodPost, loadCharacterizationRoute, strings.NewReader(`{"duration_seconds":60}`)))
	require.Equal(t, http.StatusCreated, start.Code)
	var started characterization.Snapshot
	require.NoError(t, json.NewDecoder(start.Body).Decode(&started))
	require.Equal(t, "active", started.State)

	conflict := httptest.NewRecorder()
	handler(conflict, httptest.NewRequest(http.MethodPost, loadCharacterizationRoute, strings.NewReader(`{"duration_seconds":60}`)))
	require.Equal(t, http.StatusConflict, conflict.Code)

	status := httptest.NewRecorder()
	handler(status, httptest.NewRequest(http.MethodGet, loadCharacterizationRoute, nil))
	require.Equal(t, http.StatusOK, status.Code)

	stop := httptest.NewRecorder()
	handler(stop, httptest.NewRequest(http.MethodDelete, loadCharacterizationRoute+"?session_id="+started.SessionID, nil))
	require.Equal(t, http.StatusOK, stop.Code)
	var completed characterization.Snapshot
	require.NoError(t, json.NewDecoder(stop.Body).Decode(&completed))
	require.Equal(t, "completed", completed.State)
}

func TestLoadCharacterizationRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
	}{
		{name: "short duration", body: `{"duration_seconds":0.5}`, status: http.StatusBadRequest},
		{name: "unknown field", body: `{"duration_seconds":60,"path":"secret"}`, status: http.StatusBadRequest},
		{name: "multiple values", body: `{"duration_seconds":60}{}`, status: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			loadCharacterizationHandler(characterization.NewManager(), true)(recorder, httptest.NewRequest(http.MethodPost, loadCharacterizationRoute, strings.NewReader(test.body)))
			require.Equal(t, test.status, recorder.Code)
		})
	}
}

func TestLoadCharacterizationRequiresFeatureFlag(t *testing.T) {
	recorder := httptest.NewRecorder()
	loadCharacterizationHandler(characterization.NewManager(), false)(recorder, httptest.NewRequest(http.MethodGet, loadCharacterizationRoute, nil))
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "not allowed")
}
