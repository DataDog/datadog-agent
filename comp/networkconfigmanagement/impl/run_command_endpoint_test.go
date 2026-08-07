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

func doRunCommandRequest(t *testing.T, comp *networkDeviceConfigImpl, req RunCommandRequest) (*httptest.ResponseRecorder, types.RunCommandResponse) {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)

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
		DeviceID: device.DeviceID(),
		Command:  "show version",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, resp.ErrorCode)
	assert.Empty(t, resp.ErrorMsg)
	if assert.NotNil(t, resp.CommandResult) {
		assert.Equal(t, versionOutput, resp.CommandResult.Output)
	}
}

func TestRunCommandEndpointHandler_UnknownDevice(t *testing.T) {
	comp, _ := createTestComponent(t)

	w, resp := doRunCommandRequest(t, comp, RunCommandRequest{
		DeviceID: "default:10.0.0.99",
		Command:  "show version",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, resp.CommandResult)
	assert.Equal(t, string(types.ErrNoSuchDevice), resp.ErrorCode)
	assert.NotEmpty(t, resp.ErrorMsg)
}

func TestRunCommandEndpointHandler_BadRequestBody(t *testing.T) {
	comp, _ := createTestComponent(t)

	r := httptest.NewRequest(http.MethodPost, "/agent/ncm/run-command", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	comp.RunCommandEndpointHandler()(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
