// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPARDequeueSurfacesRshellPolicyAsSystemInputs(t *testing.T) {
	fi := NewServer()
	fi.par.queue = []parQueuedTask{{
		TaskID:    "task-1",
		ActionFQN: "com.datadoghq.remoteaction.rshell.runCommand",
		Inputs: map[string]interface{}{
			"command":         "cat /tmp/file",
			"allowedCommands": []string{"rshell:cat"},
			"allowedPaths":    []string{"/tmp:rw", "/host/var/log"},
		},
	}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/on-prem-management-service/workflow-tasks/dequeue", nil)
	fi.handlePARDequeue(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	data := got["data"].(map[string]interface{})
	attributes := data["attributes"].(map[string]interface{})
	systemInputs := attributes["system_inputs"].(map[string]interface{})
	remoteAction := systemInputs["remote_action"].(map[string]interface{})
	assert.Equal(t, []interface{}{"rshell:cat"}, remoteAction["target_commands"])
	assert.Equal(t, []interface{}{"/tmp:rw", "/host/var/log"}, remoteAction["target_paths"])
}
