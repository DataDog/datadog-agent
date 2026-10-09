// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// showVersion is a command block that renders to "show version".
const showVersion = `[{"type":"show","target":"version"}]`

func doRunCommandRequest(t *testing.T, comp *networkDeviceConfigImpl, req RunCommandRequest) (*httptest.ResponseRecorder, types.RunCommandResponse) {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)
	return doRunCommandRequestBody(t, comp, body)
}

func doRunCommandRequestBody(t *testing.T, comp *networkDeviceConfigImpl, body []byte) (*httptest.ResponseRecorder, types.RunCommandResponse) {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/agent/ncm/run-command", bytes.NewReader(body))
	w := httptest.NewRecorder()
	comp.RunCommandEndpointHandler()(w, r)

	var resp types.RunCommandResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w, resp
}

func TestRunCommandEndpointHandler_Success(t *testing.T) {
	comp, reqs := createTestComponent(t)
	device := createTestDevice()
	require.NoError(t, comp.RegisterDevice(device))
	reqs.connFactory.conn.OutputMap["show version"] = ok(versionOutput)

	w, resp := doRunCommandRequest(t, comp, RunCommandRequest{
		DeviceID:      device.DeviceID(),
		Command:       json.RawMessage(showVersion),
		CredentialSet: "readonly",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, resp.ErrorCode)
	assert.Empty(t, resp.ErrorMsg)
	if assert.NotNil(t, resp.CommandResult) {
		assert.Equal(t, versionOutput, resp.CommandResult.Output)
	}
}

func TestRunCommandEndpointHandler_RollbackCredentialSetRejected(t *testing.T) {
	comp, reqs := createTestComponent(t)
	device := createTestDevice()
	require.NoError(t, comp.RegisterDevice(device))
	reqs.connFactory.conn.OutputMap["show version"] = ok(versionOutput)

	w, resp := doRunCommandRequest(t, comp, RunCommandRequest{
		DeviceID:      device.DeviceID(),
		Command:       json.RawMessage(showVersion),
		CredentialSet: "rollback",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, resp.CommandResult)
	assert.Equal(t, string(types.ErrCannotConnect), resp.ErrorCode)
	assert.NotEmpty(t, resp.ErrorMsg)
}

func TestRunCommandEndpointHandler_UnknownDevice(t *testing.T) {
	comp, _ := createTestComponent(t)

	w, resp := doRunCommandRequest(t, comp, RunCommandRequest{
		DeviceID:      "default:10.0.0.99",
		Command:       json.RawMessage(showVersion),
		CredentialSet: "readonly",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, resp.CommandResult)
	assert.Equal(t, string(types.ErrNoSuchDevice), resp.ErrorCode)
	assert.NotEmpty(t, resp.ErrorMsg)
}

func TestRunCommandEndpointHandler_UnknownCredentialSet(t *testing.T) {
	comp, _ := createTestComponent(t)
	device := createTestDevice()
	require.NoError(t, comp.RegisterDevice(device))

	w, resp := doRunCommandRequest(t, comp, RunCommandRequest{
		DeviceID:      device.DeviceID(),
		Command:       json.RawMessage(showVersion),
		CredentialSet: "superuser",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, resp.CommandResult)
	assert.Equal(t, string(types.ErrCannotConnect), resp.ErrorCode)
	assert.NotEmpty(t, resp.ErrorMsg)
}

func TestRunCommandEndpointHandler_BadRequestBody(t *testing.T) {
	comp, _ := createTestComponent(t)

	r := httptest.NewRequest(http.MethodPost, "/agent/ncm/run-command", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	comp.RunCommandEndpointHandler()(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRunCommandEndpointHandler_MultiLineBlock(t *testing.T) {
	comp, reqs := createTestComponent(t)
	device := createTestDevice()
	require.NoError(t, comp.RegisterDevice(device))
	rendered := "configure terminal\nhostname r1\nend\nshow version"
	reqs.connFactory.conn.OutputMap[rendered] = ok(versionOutput)

	w, resp := doRunCommandRequest(t, comp, RunCommandRequest{
		DeviceID:      device.DeviceID(),
		Command:       json.RawMessage(`[{"type":"hostname","hostname":"r1"},{"type":"show","target":"version"}]`),
		CredentialSet: "admin",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, resp.ErrorCode)
	assert.Empty(t, resp.ErrorMsg)
	if assert.NotNil(t, resp.CommandResult) {
		assert.Equal(t, rendered, resp.CommandResult.CommandStr)
		assert.Equal(t, versionOutput, resp.CommandResult.Output)
	}
}

func TestRunCommandEndpointHandler_InvalidCommand(t *testing.T) {
	for name, command := range map[string]string{
		"plain_string":   `"show version"`,
		"object":         `{"type":"show","target":"version"}`,
		"unknown_type":   `[{"type":"reload"}]`,
		"unknown_field":  `[{"type":"show","target":"version","extra":1}]`,
		"empty_block":    `[]`,
		"injection":      `[{"type":"show","target":"interfaces","interface":"Gi1/0/1|python$IFS-c$IFS'print(\"gotcha\")'"}]`,
		"zone_injection": `[{"type":"interface","name":"Gi1/0/1","commands":[{"type":"ipv6_addr","prefix":"fe80::1%x\nreload","modifier":"link-local"}]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			comp, reqs := createTestComponent(t)
			device := createTestDevice()
			require.NoError(t, comp.RegisterDevice(device))

			w, resp := doRunCommandRequest(t, comp, RunCommandRequest{
				DeviceID:      device.DeviceID(),
				Command:       json.RawMessage(command),
				CredentialSet: "admin",
			})

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Nil(t, resp.CommandResult)
			assert.Equal(t, string(types.ErrInvalidCommand), resp.ErrorCode)
			assert.NotEmpty(t, resp.ErrorMsg)
			assert.False(t, reqs.connFactory.conn.Opened, "should not connect to the device")
		})
	}
}

func TestRunCommandEndpointHandler_MissingCommand(t *testing.T) {
	for name, body := range map[string]string{
		"absent": `{"device_id":"default:10.0.0.1","credential_set":"readonly"}`,
		"null":   `{"device_id":"default:10.0.0.1","command":null,"credential_set":"readonly"}`,
	} {
		t.Run(name, func(t *testing.T) {
			comp, reqs := createTestComponent(t)
			device := createTestDevice()
			require.NoError(t, comp.RegisterDevice(device))

			w, resp := doRunCommandRequestBody(t, comp, []byte(body))

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Nil(t, resp.CommandResult)
			assert.Equal(t, string(types.ErrInvalidCommand), resp.ErrorCode)
			assert.False(t, reqs.connFactory.conn.Opened, "should not connect to the device")
		})
	}
}
