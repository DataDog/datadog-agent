// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestPARDequeueSurfacesRshellPolicyInSignedEnvelope(t *testing.T) {
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
	signedEnvelope := attributes["signed_envelope"].(map[string]interface{})
	signedTaskData, err := base64.StdEncoding.DecodeString(signedEnvelope["data"].(string))
	require.NoError(t, err)

	var task privateactionspb.PrivateActionTask
	require.NoError(t, proto.Unmarshal(signedTaskData, &task))

	remoteAction := task.GetSystemInputs().GetRemoteAction()
	require.NotNil(t, remoteAction)
	assert.Equal(t, []string{"rshell:cat"}, remoteAction.AllowedCommands)
	assert.Equal(t, []string{"/tmp:rw", "/host/var/log"}, remoteAction.AllowedPaths)

	assert.Equal(t, map[string]interface{}{"command": "cat /tmp/file"}, task.Inputs.AsMap())
	assert.Equal(t, map[string]interface{}{"command": "cat /tmp/file"}, attributes["inputs"])
}

func TestPARDequeueSurfacesEmptyRshellPolicyInSignedEnvelope(t *testing.T) {
	fi := NewServer()
	fi.par.queue = []parQueuedTask{{
		TaskID:    "task-1",
		ActionFQN: "com.datadoghq.remoteaction.rshell.runCommand",
		Inputs: map[string]interface{}{
			"command":         "cat /tmp/file",
			"allowedCommands": []string{},
			"allowedPaths":    []string{},
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
	signedEnvelope := attributes["signed_envelope"].(map[string]interface{})
	signedTaskData, err := base64.StdEncoding.DecodeString(signedEnvelope["data"].(string))
	require.NoError(t, err)

	var task privateactionspb.PrivateActionTask
	require.NoError(t, proto.Unmarshal(signedTaskData, &task))

	remoteAction := task.GetSystemInputs().GetRemoteAction()
	require.NotNil(t, remoteAction)
	assert.Empty(t, remoteAction.AllowedCommands)
	assert.Empty(t, remoteAction.AllowedPaths)
	assert.Equal(t, map[string]interface{}{"command": "cat /tmp/file"}, task.Inputs.AsMap())
}

func TestPARDequeueLeavesLegacyAllowedPathsInputInSignedEnvelope(t *testing.T) {
	fi := NewServer()
	legacyAllowedPaths := map[string]interface{}{"default": []string{"/tmp"}}
	fi.par.queue = []parQueuedTask{{
		TaskID:    "task-1",
		ActionFQN: "com.datadoghq.remoteaction.rshell.runCommand",
		Inputs: map[string]interface{}{
			"command":      "cat /tmp/file",
			"allowedPaths": legacyAllowedPaths,
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
	signedEnvelope := attributes["signed_envelope"].(map[string]interface{})
	signedTaskData, err := base64.StdEncoding.DecodeString(signedEnvelope["data"].(string))
	require.NoError(t, err)

	var task privateactionspb.PrivateActionTask
	require.NoError(t, proto.Unmarshal(signedTaskData, &task))

	assert.Nil(t, task.GetSystemInputs())
	assert.Equal(t, map[string]interface{}{
		"command": "cat /tmp/file",
		"allowedPaths": map[string]interface{}{
			"default": []interface{}{"/tmp"},
		},
	}, task.Inputs.AsMap())
}
