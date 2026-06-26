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

func TestPARDequeueSurfacesAllowedPathsAsTargetPaths(t *testing.T) {
	cases := []struct {
		name         string
		allowedPaths interface{}
		wantPaths    []interface{}
	}{
		{
			name:         "flat path list",
			allowedPaths: []string{"/tmp:rw", "/host/var/log"},
			wantPaths:    []interface{}{"/tmp:rw", "/host/var/log"},
		},
		{
			name: "legacy containerized path map",
			allowedPaths: map[string]interface{}{
				"default":       []interface{}{"/var/log"},
				"containerized": []interface{}{"/host/var/log:rw"},
			},
			wantPaths: []interface{}{"/host/var/log:rw"},
		},
		{
			name: "legacy default path map",
			allowedPaths: map[string]interface{}{
				"default": []interface{}{"/var/log"},
			},
			wantPaths: []interface{}{"/var/log"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fi := NewServer()
			fi.par.queue = []parQueuedTask{{
				TaskID:    "task-1",
				ActionFQN: "com.datadoghq.remoteaction.rshell.runCommand",
				Inputs: map[string]interface{}{
					"command":         "cat /tmp/file",
					"allowedCommands": []string{"rshell:cat"},
					"allowedPaths":    tc.allowedPaths,
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
			remoteAction := attributes["remote_action"].(map[string]interface{})
			assert.Equal(t, []interface{}{"rshell:cat"}, remoteAction["target_commands"])
			assert.Equal(t, tc.wantPaths, remoteAction["target_paths"])
		})
	}
}
