// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkdevices

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestConnectivityCheckHandlerRun(t *testing.T) {
	cases := []struct {
		name            string
		taskInputs      map[string]any
		expectedResults interface{}
		expectedErr     string
	}{
		{
			name: "fails when inputs are malformed",
			taskInputs: map[string]any{
				"targetIPs": "not-a-list",
			},
			expectedErr: "failed to parse connectivityCheck inputs",
		},
		{
			name: "fails when secret inputs cannot be decrypted",
			taskInputs: map[string]any{
				"targetIPs":            []string{},
				"checks":               []string{},
				"encryptedCredentials": "not-decryptable",
			},
			expectedErr: "failed to decrypt secret inputs",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewConnectivityCheckHandler(encryptioncontext.NewStore(), ipcmock.New(t).GetClient())

			res, err := handler.Run(context.Background(), makeConnectivityTask(tc.taskInputs), nil)
			require.Equal(t, tc.expectedResults, res)
			require.ErrorContains(t, err, tc.expectedErr)
		})
	}
}

func makeConnectivityTask(taskInputs map[string]any) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{Inputs: taskInputs}
	return task
}

type checkingIPCClient struct {
	ipc.HTTPClient
	check func(context.Context)
}

func (c checkingIPCClient) Do(req *http.Request, _ ...ipc.RequestOption) ([]byte, error) {
	c.check(req.Context())
	return []byte(`{"devices":[]}`), nil
}

func TestConnectivityCheckHandlerTimeout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		timeout any
		want    time.Duration
	}{
		{name: "missing timeout inherits parent", want: time.Minute},
		{name: "zero timeout inherits parent", timeout: 0, want: time.Minute},
		{name: "negative timeout inherits parent", timeout: -1, want: time.Minute},
		{name: "shorter timeout applies in seconds", timeout: 5, want: 5 * time.Second},
		{name: "longer timeout cannot extend parent", timeout: 120, want: time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := ipcmock.New(t).GetClient()
			parentDeadline := time.Now().Add(time.Minute)
			ctx, cancel := context.WithDeadline(context.Background(), parentDeadline)
			defer cancel()
			before := time.Now()
			handler := NewConnectivityCheckHandler(encryptioncontext.NewStore(), checkingIPCClient{
				HTTPClient: client,
				check: func(ctx context.Context) {
					deadline, ok := ctx.Deadline()
					require.True(t, ok)
					if tc.want == time.Minute {
						require.Equal(t, parentDeadline, deadline)
					} else {
						require.False(t, deadline.Before(before.Add(tc.want)))
						require.False(t, deadline.After(time.Now().Add(tc.want)))
					}
				},
			})
			inputs := map[string]any{"targetIPs": []string{"127.0.0.1"}, "checks": []string{"ping"}}
			if tc.timeout != nil {
				inputs["timeout"] = tc.timeout
			}
			_, err := handler.Run(ctx, makeConnectivityTask(inputs), nil)
			require.NoError(t, err)
		})
	}
}

type recordingIPCClient struct {
	ipc.HTTPClient
	body     []byte
	response []byte
}

func (c *recordingIPCClient) Do(req *http.Request, _ ...ipc.RequestOption) ([]byte, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	c.body = body
	return c.response, nil
}

func TestConnectivityCheckHandlerForwardsTargetGroups(t *testing.T) {
	client := &recordingIPCClient{
		HTTPClient: ipcmock.New(t).GetClient(),
		response:   []byte(`{"devices":[{"ipAddress":"10.0.0.1","groupID":"group-1"}]}`),
	}
	handler := NewConnectivityCheckHandler(encryptioncontext.NewStore(), client)

	res, err := handler.Run(context.Background(), makeConnectivityTask(map[string]any{
		"targetIPs": []string{"10.0.0.1"},
		"checks":    []string{"snmp"},
		"targetGroups": []map[string]any{
			{"id": "group-1", "targetIPs": []string{"10.0.0.1"}, "credentialIDs": []string{"cred-1"}},
		},
	}), nil)
	require.NoError(t, err)

	var sent connectivity.Request
	require.NoError(t, json.Unmarshal(client.body, &sent))
	require.Equal(t, []connectivity.TargetGroup{
		{ID: "group-1", TargetIPs: []string{"10.0.0.1"}, CredentialIDs: []string{"cred-1"}},
	}, sent.TargetGroups)
	require.Equal(t, &connectivity.Result{Devices: []connectivity.DeviceResult{{IPAddress: "10.0.0.1", GroupID: "group-1"}}}, res)
}

func TestConnectivityCheckHandlerSendsNoTargetGroupsForALegacyRequest(t *testing.T) {
	client := &recordingIPCClient{HTTPClient: ipcmock.New(t).GetClient(), response: []byte(`{"devices":[]}`)}
	handler := NewConnectivityCheckHandler(encryptioncontext.NewStore(), client)

	_, err := handler.Run(context.Background(), makeConnectivityTask(map[string]any{
		"targetIPs": []string{"10.0.0.1"},
		"checks":    []string{"ping"},
	}), nil)
	require.NoError(t, err)

	var sent map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(client.body, &sent))
	require.JSONEq(t, "null", string(sent["targetGroups"]), "a nil list keeps the request ungrouped")
}

func TestConnectivityCheckHandlerKeepsAnEmptyTargetGroupListGrouped(t *testing.T) {
	client := &recordingIPCClient{HTTPClient: ipcmock.New(t).GetClient(), response: []byte(`{"devices":[]}`)}
	handler := NewConnectivityCheckHandler(encryptioncontext.NewStore(), client)

	_, err := handler.Run(context.Background(), makeConnectivityTask(map[string]any{
		"targetIPs":    []string{"10.0.0.1"},
		"checks":       []string{"ping"},
		"targetGroups": []map[string]any{},
	}), nil)
	require.NoError(t, err)

	var sent map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(client.body, &sent))
	require.JSONEq(t, "[]", string(sent["targetGroups"]), "the Agent must reject it rather than try every credential")
}
