// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkconfigmanagement

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ncmtypes "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// fakeIPCClient is a minimal ipc.HTTPClient implementation for testing PAR
// handlers without a real HTTP round-trip.
type fakeIPCClient struct {
	postResp []byte
	postErr  error
	// postURL and postBody record the most recent Post call.
	postURL  string
	postBody []byte
}

var _ ipc.HTTPClient = (*fakeIPCClient)(nil)

func (f *fakeIPCClient) Do(_ *http.Request, _ ...ipc.RequestOption) ([]byte, error) {
	return f.postResp, f.postErr
}

func (f *fakeIPCClient) Get(_ string, _ ...ipc.RequestOption) ([]byte, error) {
	return f.postResp, f.postErr
}

func (f *fakeIPCClient) Head(_ string, _ ...ipc.RequestOption) ([]byte, error) {
	return f.postResp, f.postErr
}

func (f *fakeIPCClient) Post(url string, _ string, body io.Reader, _ ...ipc.RequestOption) ([]byte, error) {
	f.postURL = url
	b, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	f.postBody = b
	return f.postResp, f.postErr
}

func (f *fakeIPCClient) PostChunk(_ string, _ string, _ io.Reader, _ func([]byte), _ ...ipc.RequestOption) error {
	return f.postErr
}

func (f *fakeIPCClient) PostForm(_ string, _ url.Values, _ ...ipc.RequestOption) ([]byte, error) {
	return f.postResp, f.postErr
}

func (f *fakeIPCClient) NewIPCEndpoint(_ string) (ipc.Endpoint, error) {
	return nil, errors.New("not implemented")
}

// showVersion is a command block that renders to "show version".
var showVersion = []any{map[string]any{"type": "show version"}}

func makeRunIOSCommandsTask(deviceID string, command any, credentialSet string) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]any{
			"deviceID":      deviceID,
			"command":       command,
			"credentialSet": credentialSet,
		},
	}
	return task
}

func TestRunIOSCommandsHandler_Success(t *testing.T) {
	resp := ncmtypes.RunIOSCommandsResponse{
		CommandResult: &ncmtypes.CommandResult{Output: "Cisco Device Version 1.0"},
	}
	body, err := json.Marshal(resp)
	require.NoError(t, err)

	client := &fakeIPCClient{postResp: body}
	handler := NewRunIOSCommandsHandler(client)

	out, err := handler.Run(t.Context(), makeRunIOSCommandsTask("default:10.0.0.1", showVersion, "readonly"), nil)
	require.NoError(t, err)

	result, ok := out.(RunIOSCommandsOutputs)
	require.True(t, ok)
	assert.True(t, result.Success)
	assert.Empty(t, result.ErrorCode)
	assert.Empty(t, result.Error)
	require.NotNil(t, result.CommandResult)
	assert.Equal(t, "Cisco Device Version 1.0", result.CommandResult.Output)
	assert.NotNil(t, result.FinishedAt)
}

func TestRunIOSCommandsHandler_DeviceError(t *testing.T) {
	resp := ncmtypes.RunIOSCommandsResponse{
		ErrorCode: string(ncmtypes.ErrNoSuchDevice),
		ErrorMsg:  `unknown device: "default:10.0.0.99"`,
	}
	body, err := json.Marshal(resp)
	require.NoError(t, err)

	client := &fakeIPCClient{postResp: body}
	handler := NewRunIOSCommandsHandler(client)

	out, err := handler.Run(t.Context(), makeRunIOSCommandsTask("default:10.0.0.99", showVersion, "readonly"), nil)
	require.NoError(t, err)

	result, ok := out.(RunIOSCommandsOutputs)
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, string(ncmtypes.ErrNoSuchDevice), result.ErrorCode)
	assert.Equal(t, `unknown device: "default:10.0.0.99"`, result.Error)
	assert.Nil(t, result.CommandResult)
}

func TestRunIOSCommandsHandler_PassesCommandThrough(t *testing.T) {
	body, err := json.Marshal(ncmtypes.RunIOSCommandsResponse{CommandResult: &ncmtypes.CommandResult{}})
	require.NoError(t, err)
	client := &fakeIPCClient{postResp: body}
	handler := NewRunIOSCommandsHandler(client)

	// The PAR doesn't validate the command, so even an invalid block is
	// forwarded unchanged for the agent to reject.
	command := []any{
		map[string]any{"type": "hostname", "hostname": "r1"},
		map[string]any{"type": "bogus", "extra": []any{1.0, "x"}},
	}
	_, err = handler.Run(t.Context(), makeRunIOSCommandsTask("default:10.0.0.1", command, "admin"), nil)
	require.NoError(t, err)

	assert.True(t, strings.HasSuffix(client.postURL, "/agent/ncm/run-ios-commands"), client.postURL)
	assert.JSONEq(t, `{
		"device_id": "default:10.0.0.1",
		"command": [{"type":"hostname","hostname":"r1"},{"type":"bogus","extra":[1,"x"]}],
		"credential_set": "admin"
	}`, string(client.postBody))
}

func TestRunIOSCommandsHandler_MissingCommand(t *testing.T) {
	for name, inputs := range map[string]map[string]any{
		"absent": {"deviceID": "default:10.0.0.1", "credentialSet": "readonly"},
		"null":   {"deviceID": "default:10.0.0.1", "command": nil, "credentialSet": "readonly"},
	} {
		t.Run(name, func(t *testing.T) {
			client := &fakeIPCClient{}
			handler := NewRunIOSCommandsHandler(client)
			task := &types.Task{}
			task.Data.Attributes = &types.Attributes{Inputs: inputs}

			_, err := handler.Run(t.Context(), task, nil)
			assert.ErrorContains(t, err, "Command input is required")
			assert.Nil(t, client.postBody, "should not call the agent")
		})
	}
}

func TestRunIOSCommandsHandler_NoIPCClient(t *testing.T) {
	handler := NewRunIOSCommandsHandler(nil)

	_, err := handler.Run(t.Context(), makeRunIOSCommandsTask("default:10.0.0.1", showVersion, "readonly"), nil)
	assert.ErrorContains(t, err, "IPC client is not available")
}
