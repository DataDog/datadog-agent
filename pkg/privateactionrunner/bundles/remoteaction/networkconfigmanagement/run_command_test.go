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

func (f *fakeIPCClient) Post(_ string, _ string, _ io.Reader, _ ...ipc.RequestOption) ([]byte, error) {
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

func makeRunCommandTask(deviceID, command string) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]any{
			"deviceID": deviceID,
			"command":  command,
		},
	}
	return task
}

func TestRunCommandHandler_Success(t *testing.T) {
	resp := ncmtypes.RunCommandResponse{
		CommandResult: &ncmtypes.CommandResult{Output: "Cisco Device Version 1.0"},
	}
	body, err := json.Marshal(resp)
	require.NoError(t, err)

	client := &fakeIPCClient{postResp: body}
	handler := NewRunCommandHandler(client)

	out, err := handler.Run(t.Context(), makeRunCommandTask("default:10.0.0.1", "show version"), nil)
	require.NoError(t, err)

	result, ok := out.(RunCommandOutputs)
	require.True(t, ok)
	assert.True(t, result.Success)
	assert.Empty(t, result.ErrorCode)
	assert.Empty(t, result.Error)
	require.NotNil(t, result.CommandResult)
	assert.Equal(t, "Cisco Device Version 1.0", result.CommandResult.Output)
	assert.NotNil(t, result.FinishedAt)
}

func TestRunCommandHandler_DeviceError(t *testing.T) {
	resp := ncmtypes.RunCommandResponse{
		ErrorCode: string(ncmtypes.ErrNoSuchDevice),
		ErrorMsg:  `unknown device: "default:10.0.0.99"`,
	}
	body, err := json.Marshal(resp)
	require.NoError(t, err)

	client := &fakeIPCClient{postResp: body}
	handler := NewRunCommandHandler(client)

	out, err := handler.Run(t.Context(), makeRunCommandTask("default:10.0.0.99", "show version"), nil)
	require.NoError(t, err)

	result, ok := out.(RunCommandOutputs)
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, string(ncmtypes.ErrNoSuchDevice), result.ErrorCode)
	assert.Equal(t, `unknown device: "default:10.0.0.99"`, result.Error)
	assert.Nil(t, result.CommandResult)
}

func TestRunCommandHandler_MissingCommand(t *testing.T) {
	client := &fakeIPCClient{}
	handler := NewRunCommandHandler(client)

	_, err := handler.Run(t.Context(), makeRunCommandTask("default:10.0.0.1", ""), nil)
	assert.ErrorContains(t, err, "Command input is required")
}

func TestRunCommandHandler_NoIPCClient(t *testing.T) {
	handler := NewRunCommandHandler(nil)

	_, err := handler.Run(t.Context(), makeRunCommandTask("default:10.0.0.1", "show version"), nil)
	assert.ErrorContains(t, err, "IPC client is not available")
}
