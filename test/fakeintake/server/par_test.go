// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
			"systemServices": map[string]interface{}{
				"nginx.service": []interface{}{"read", "restart"},
			},
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
	require.Contains(t, remoteAction.SystemServices, "nginx.service")
	assert.Equal(t, []interface{}{"read", "restart"}, remoteAction.SystemServices["nginx.service"].AsSlice())

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
			"systemServices":  map[string]interface{}{},
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
	assert.Empty(t, remoteAction.SystemServices)
	assert.Equal(t, map[string]interface{}{"command": "cat /tmp/file"}, task.Inputs.AsMap())
	assert.Equal(t, map[string]interface{}{"command": "cat /tmp/file"}, attributes["inputs"])
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

func TestPARSetSigningKeyProducesVerifiableSignedEnvelope(t *testing.T) {
	fi := NewServer()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	keyReq, err := json.Marshal(map[string]interface{}{
		"key_id":        "test-key",
		"private_key":   []byte(priv),
		"org_id":        123456,
		"runner_id":     "test-runner-e2e",
		"connection_id": "connection:execgroup_ddagent:par-rshell-e2e",
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/fakeintake/par/signing-key", bytes.NewReader(keyReq))
	fi.handlePARSetSigningKey(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	fi.par.queue = []parQueuedTask{{
		TaskID:    "task-1",
		ActionFQN: "com.datadoghq.remoteaction.rshell.runCommand",
		Inputs:    map[string]interface{}{"command": "cat /tmp/file"},
	}}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v2/on-prem-management-service/workflow-tasks/dequeue", nil)
	fi.handlePARDequeue(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	attributes := got["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	signedEnvelope := attributes["signed_envelope"].(map[string]interface{})

	signedTaskData, err := base64.StdEncoding.DecodeString(signedEnvelope["data"].(string))
	require.NoError(t, err)

	assert.Equal(t, float64(privateactionspb.HashType_SHA256), signedEnvelope["hash_type"])
	signatures := signedEnvelope["signatures"].([]interface{})
	require.Len(t, signatures, 1)
	signature := signatures[0].(map[string]interface{})
	assert.Equal(t, "test-key", signature["key_id"])
	assert.Equal(t, float64(privateactionspb.KeyType_ED25519), signature["key_type"])

	sig, err := base64.StdEncoding.DecodeString(signature["signature"].(string))
	require.NoError(t, err)
	hashedPayload := sha256.Sum256(signedTaskData)
	assert.True(t, ed25519.Verify(pub, hashedPayload[:], sig), "signature should verify against the registered public key")

	var task privateactionspb.PrivateActionTask
	require.NoError(t, proto.Unmarshal(signedTaskData, &task))
	assert.EqualValues(t, 123456, task.OrgId)
	assert.Equal(t, "connection:execgroup_ddagent:par-rshell-e2e", task.GetConnectionInfo().GetConnectionId())
	assert.Equal(t, privateactionspb.CredentialsType_TOKEN_AUTH, task.GetConnectionInfo().GetCredentialsType())
	assert.Equal(t, "test-runner-e2e", task.GetConnectionInfo().GetRunnerId())
	require.NotNil(t, task.ExpirationTime)
	assert.True(t, task.ExpirationTime.AsTime().After(time.Now()))
}

func TestPARSetSigningKeyRejectsInvalidPrivateKeySize(t *testing.T) {
	fi := NewServer()

	keyReq, err := json.Marshal(map[string]interface{}{
		"key_id":        "test-key",
		"private_key":   []byte("too-short"),
		"org_id":        1,
		"runner_id":     "runner",
		"connection_id": "connection:execgroup_ddagent:test-host",
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/fakeintake/par/signing-key", bytes.NewReader(keyReq))
	fi.handlePARSetSigningKey(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
