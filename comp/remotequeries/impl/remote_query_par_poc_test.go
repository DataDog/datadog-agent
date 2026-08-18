// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

func TestRemoteQueryPARHarnessUsesCredentialFreeIPCPostShape(t *testing.T) {
	client := &capturePostClient{response: []byte(`{"status":"SUCCEEDED","rows":[{"value":1}]}`)}
	harness := NewRemoteQueryPARHarness(client, "https://localhost:5001"+AgentRemoteQueryExecuteEndpointPath)

	result, err := harness.Execute(context.Background(), RemoteQueryPARInputs{
		Integration: "postgres",
		Operation:   "copy_stream",
		Format:      "csv",
		Target:      remoteQueryTargetJSON{Host: "localhost", Port: 5432, DBName: "postgres"},
		Query:       remoteQueryProofSeedQuery,
		CopyLimits:  &remoteQueryExecuteCopyLimitsJSON{ChunkBytes: 1024, MaxBytes: 1024, MaxRowBytes: 1024, TimeoutMs: 1000},
	})

	require.NoError(t, err)
	assert.Equal(t, "https://localhost:5001"+AgentRemoteQueryExecuteEndpointPath, client.url)
	assert.Equal(t, "application/json", client.contentType)
	assert.JSONEq(t, `{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","copyLimits":{"chunkBytes":1024,"maxBytes":1024,"maxRowBytes":1024,"timeoutMs":1000}}`, client.body)
	assert.NotContains(t, client.body, "password")
	assert.NotContains(t, client.body, "secret")
	assert.Equal(t, "SUCCEEDED", result.Status)
	assert.JSONEq(t, `{"status":"SUCCEEDED","rows":[{"value":1}]}`, string(result.Raw))
}

func TestRemoteQueryPARHarnessWithRealAgentIPCClientRejectsHTTPExecution(t *testing.T) {
	handler := &remoteQueryExecuteHandler{enabled: true, collector: fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: &fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: datastore-secret\n"}}}}}}
	ipc := ipcmock.New(t)
	mux := http.NewServeMux()
	mux.HandleFunc(AgentRemoteQueryExecuteEndpointPath, handler.handle)
	server := ipc.NewMockServer(ipc.HTTPMiddleware(mux))
	harness := NewRemoteQueryPARHarness(ipc.GetClient(), server.URL+AgentRemoteQueryExecuteEndpointPath)

	result, err := harness.Execute(context.Background(), RemoteQueryPARInputs{
		Integration: "postgres",
		Operation:   "copy_stream",
		Format:      "csv",
		Target:      remoteQueryTargetJSON{Host: "LOCALHOST.", Port: 5432, DBName: "postgres"},
		Query:       remoteQueryProofSeedQuery,
	})

	require.NoError(t, err)
	assert.Equal(t, statusInvalidRequest, result.Status)
	require.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Message, "streaming executor")
	assert.NotContains(t, string(result.Raw), "datastore-secret")
}

func TestRemoteQueryPARHarnessPropagatesSanitizedBridgeErrors(t *testing.T) {
	handler := &remoteQueryExecuteHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
		&fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: datastore-secret\n"}},
	}}}
	ipc := ipcmock.New(t)
	mux := http.NewServeMux()
	mux.HandleFunc(AgentRemoteQueryExecuteEndpointPath, handler.handle)
	server := ipc.NewMockServer(ipc.HTTPMiddleware(mux))
	harness := NewRemoteQueryPARHarness(ipc.GetClient(), server.URL+AgentRemoteQueryExecuteEndpointPath)

	tests := []struct {
		name       string
		inputs     RemoteQueryPARInputs
		wantStatus string
		wantCode   string
	}{
		{
			name: "target not found",
			inputs: RemoteQueryPARInputs{
				Integration: "postgres",
				Operation:   "copy_stream",
				Format:      "csv",
				Target:      remoteQueryTargetJSON{Host: "localhost", Port: 5432, DBName: "other"},
				Query:       remoteQueryProofSeedQuery,
			},
			wantStatus: statusInvalidRequest,
			wantCode:   statusInvalidRequest,
		},
		{
			name: "invalid query",
			inputs: RemoteQueryPARInputs{
				Integration: "postgres",
				Operation:   "copy_stream",
				Format:      "csv",
				Target:      remoteQueryTargetJSON{Host: "localhost", Port: 5432, DBName: "postgres"},
				Query:       "SELECT 2 AS value",
			},
			wantStatus: statusInvalidRequest,
			wantCode:   statusInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := harness.Execute(context.Background(), tt.inputs)

			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, result.Status)
			require.NotNil(t, result.Error)
			assert.Equal(t, tt.wantCode, result.Error.Code)
			assert.NotContains(t, string(result.Raw), "datastore-secret")
			assert.NotContains(t, string(result.Raw), tt.inputs.Target.DBName)
			assert.NotContains(t, string(result.Raw), tt.inputs.Query)
		})
	}
}

type capturePostClient struct {
	response    []byte
	err         error
	url         string
	contentType string
	body        string
}

func (c *capturePostClient) Post(url string, contentType string, body io.Reader, _ ...ipc.RequestOption) ([]byte, error) {
	c.url = url
	c.contentType = contentType
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	c.body = string(payload)
	return c.response, c.err
}

var _ remoteQueryPARIPCClient = (*capturePostClient)(nil)
