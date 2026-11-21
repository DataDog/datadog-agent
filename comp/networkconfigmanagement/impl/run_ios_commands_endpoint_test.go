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

	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	ncmprofile "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// showVersion is a command block that renders to "show version".
const showVersion = `[{"type":"show version"}]`

func createTestIOSDevice() *ncmconfig.DeviceInstance {
	device := createTestDevice()
	device.Profile = string(ncmprofile.ProfileCiscoIOS)
	return device
}

func doRunIOSCommandsRequest(t *testing.T, comp *networkDeviceConfigImpl, req RunIOSCommandsRequest) (*httptest.ResponseRecorder, types.RunIOSCommandsResponse) {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)
	return doRunIOSCommandsRequestBody(t, comp, body)
}

func doRunIOSCommandsRequestBody(t *testing.T, comp *networkDeviceConfigImpl, body []byte) (*httptest.ResponseRecorder, types.RunIOSCommandsResponse) {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/agent/ncm/run-ios-commands", bytes.NewReader(body))
	w := httptest.NewRecorder()
	comp.RunIOSCommandsEndpointHandler()(w, r)

	var resp types.RunIOSCommandsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w, resp
}

func TestRunIOSCommandsEndpointHandler_Success(t *testing.T) {
	comp, reqs := createTestComponent(t)
	device := createTestIOSDevice()
	require.NoError(t, comp.RegisterDevice(device))
	reqs.connFactory.conn.OutputMap["show version"] = ok(versionOutput)

	w, resp := doRunIOSCommandsRequest(t, comp, RunIOSCommandsRequest{
		DeviceID: device.DeviceID(),
		Command:  json.RawMessage(showVersion),
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, resp.ErrorCode)
	assert.Empty(t, resp.ErrorMsg)
	assert.Equal(t, []*types.CommandResult{
		{CommandStr: "show version", Output: versionOutput},
	}, resp.CommandResults)
}

func TestRunIOSCommandsEndpointHandler_NonIOSDevice(t *testing.T) {
	comp, reqs := createTestComponent(t)
	device := createTestDevice()
	require.NoError(t, comp.RegisterDevice(device))
	reqs.connFactory.conn.OutputMap["show version"] = ok(versionOutput)

	w, resp := doRunIOSCommandsRequest(t, comp, RunIOSCommandsRequest{
		DeviceID: device.DeviceID(),
		Command:  json.RawMessage(showVersion),
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, resp.CommandResults)
	assert.Equal(t, string(types.ErrCommandUnsupported), resp.ErrorCode)
	assert.NotEmpty(t, resp.ErrorMsg)
}

func TestRunIOSCommandsEndpointHandler_UnknownDevice(t *testing.T) {
	comp, _ := createTestComponent(t)

	w, resp := doRunIOSCommandsRequest(t, comp, RunIOSCommandsRequest{
		DeviceID: "default:10.0.0.99",
		Command:  json.RawMessage(showVersion),
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, resp.CommandResults)
	assert.Equal(t, string(types.ErrNoSuchDevice), resp.ErrorCode)
	assert.NotEmpty(t, resp.ErrorMsg)
}

func TestRunIOSCommandsEndpointHandler_BadRequestBody(t *testing.T) {
	comp, _ := createTestComponent(t)

	r := httptest.NewRequest(http.MethodPost, "/agent/ncm/run-ios-commands", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	comp.RunIOSCommandsEndpointHandler()(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRunIOSCommandsEndpointHandler_MultiLineBlock(t *testing.T) {
	comp, reqs := createTestComponent(t)
	device := createTestIOSDevice()
	require.NoError(t, comp.RegisterDevice(device))
	reqs.connFactory.conn.OutputMap["show version"] = ok(versionOutput)

	w, resp := doRunIOSCommandsRequest(t, comp, RunIOSCommandsRequest{
		DeviceID: device.DeviceID(),
		Command:  json.RawMessage(`[{"type":"hostname","hostname":"r1"},{"type":"show version"}]`),
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, resp.ErrorCode)
	assert.Empty(t, resp.ErrorMsg)
	assert.Equal(t, []*types.CommandResult{
		{CommandStr: "configure terminal"},
		{CommandStr: "hostname r1"},
		{CommandStr: "end"},
		{CommandStr: "show version", Output: versionOutput},
	}, resp.CommandResults)
}

func TestRunIOSCommandsEndpointHandler_StopsAtFailedLine(t *testing.T) {
	comp, reqs := createTestComponent(t)
	device := createTestIOSDevice()
	require.NoError(t, comp.RegisterDevice(device))
	conn := reqs.connFactory.conn
	conn.OutputMap["hostname r1"] = fail("rejected")

	w, resp := doRunIOSCommandsRequest(t, comp, RunIOSCommandsRequest{
		DeviceID: device.DeviceID(),
		Command:  json.RawMessage(`[{"type":"hostname","hostname":"r1"},{"type":"show version"}]`),
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, string(types.ErrCommandFailed), resp.ErrorCode)
	assert.Contains(t, resp.ErrorMsg, "Invalid input")
	require.Len(t, resp.CommandResults, 2)
	assert.Equal(t, "hostname r1", resp.CommandResults[1].CommandStr)
	assert.Equal(t, "% Invalid input detected at '^' marker.", resp.CommandResults[1].Error)
	// The run stops at the failed line and leaves config mode.
	assert.Equal(t, []string{"configure terminal", "hostname r1", "end"}, conn.Calls)
}

func TestRunIOSCommandsEndpointHandler_InvalidCommand(t *testing.T) {
	for name, command := range map[string]string{
		"plain_string":   `"show version"`,
		"object":         `{"type":"show version"}`,
		"unknown_type":   `[{"type":"reload"}]`,
		"unknown_field":  `[{"type":"show version","extra":1}]`,
		"empty_block":    `[]`,
		"injection":      `[{"type":"show interfaces","interface":"Gi1/0/1|python$IFS-c$IFS'print(\"gotcha\")'"}]`,
		"zone_injection": `[{"type":"interface","name":"Gi1/0/1","commands":[{"type":"ipv6 address","prefix":"fe80::1%x\nreload","modifier":"link-local"}]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			comp, reqs := createTestComponent(t)
			device := createTestIOSDevice()
			require.NoError(t, comp.RegisterDevice(device))

			w, resp := doRunIOSCommandsRequest(t, comp, RunIOSCommandsRequest{
				DeviceID: device.DeviceID(),
				Command:  json.RawMessage(command),
			})

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Empty(t, resp.CommandResults)
			assert.Equal(t, string(types.ErrInvalidCommand), resp.ErrorCode)
			assert.NotEmpty(t, resp.ErrorMsg)
			assert.False(t, reqs.connFactory.conn.Opened, "should not connect to the device")
		})
	}
}

func TestRunIOSCommandsEndpointHandler_MissingCommand(t *testing.T) {
	for name, body := range map[string]string{
		"absent": `{"device_id":"default:10.0.0.1"}`,
		"null":   `{"device_id":"default:10.0.0.1","command":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			comp, reqs := createTestComponent(t)
			device := createTestIOSDevice()
			require.NoError(t, comp.RegisterDevice(device))

			w, resp := doRunIOSCommandsRequestBody(t, comp, []byte(body))

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Empty(t, resp.CommandResults)
			assert.Equal(t, string(types.ErrInvalidCommand), resp.ErrorCode)
			assert.False(t, reqs.connFactory.conn.Opened, "should not connect to the device")
		})
	}
}

func TestRunIOSCommandsEndpointHandler_RedactsOutput(t *testing.T) {
	comp, reqs := createTestComponent(t)
	device := createTestIOSDevice()
	require.NoError(t, comp.RegisterDevice(device))
	reqs.connFactory.conn.OutputMap["show running-config"] = ok("hostname r1\nenable secret 5 $1$abc$xyz\n")

	w, resp := doRunIOSCommandsRequest(t, comp, RunIOSCommandsRequest{
		DeviceID: device.DeviceID(),
		Command:  json.RawMessage(`[{"type":"show running-config"}]`),
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, resp.ErrorCode)
	if assert.Len(t, resp.CommandResults, 1) {
		assert.Contains(t, resp.CommandResults[0].Output, "enable secret 5 <secret hidden>")
		assert.NotContains(t, resp.CommandResults[0].Output, "$1$abc$xyz")
	}
}

func TestRunIOSCommandsEndpointHandler_RedactsFailedOutput(t *testing.T) {
	comp, reqs := createTestComponent(t)
	device := createTestIOSDevice()
	require.NoError(t, comp.RegisterDevice(device))
	reqs.connFactory.conn.OutputMap["show running-config"] = &result{
		Output: "enable secret 5 $1$abc$xyz",
		Error:  "rejected",
	}

	w, resp := doRunIOSCommandsRequest(t, comp, RunIOSCommandsRequest{
		DeviceID: device.DeviceID(),
		Command:  json.RawMessage(`[{"type":"show running-config"}]`),
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, string(types.ErrCommandFailed), resp.ErrorCode)
	assert.Contains(t, resp.ErrorMsg, "Invalid input")
	assert.Contains(t, resp.ErrorMsg, "<secret hidden>")
	assert.NotContains(t, resp.ErrorMsg, "$1$abc$xyz")
	if assert.Len(t, resp.CommandResults, 1) {
		assert.Contains(t, resp.CommandResults[0].Output, "enable secret 5 <secret hidden>")
		assert.NotContains(t, resp.CommandResults[0].Output, "$1$abc$xyz")
	}
}
