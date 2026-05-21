// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remotequeries

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteActionUsesCredentialFreeIPCPostShape(t *testing.T) {
	client := &captureBridgeClient{response: []byte(`{"status":"SUCCEEDED","rows":[{"value":1}]}`)}
	action := NewExecuteAction(func() (BridgeClient, string, error) {
		return client, "https://localhost:5001" + AgentRemoteQueryExecuteEndpointPath, nil
	})

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target": map[string]interface{}{
			"host":   "localhost",
			"port":   5432,
			"dbname": "postgres",
		},
		"query": "SELECT 1 AS value",
		"limits": map[string]interface{}{
			"maxRows":   1,
			"maxBytes":  1024,
			"timeoutMs": 1000,
		},
	}), &privateconnection.PrivateCredentials{Tokens: []privateconnection.PrivateCredentialsToken{{Name: "password", Value: "secret-value"}}})

	require.NoError(t, err)
	assert.Equal(t, "https://localhost:5001"+AgentRemoteQueryExecuteEndpointPath, client.url)
	assert.Equal(t, "application/json", client.contentType)
	assert.JSONEq(t, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","limits":{"maxRows":1,"maxBytes":1024,"timeoutMs":1000}}`, client.body)
	assert.NotContains(t, client.body, "secret-value")
	assert.Equal(t, map[string]interface{}{
		"status": "SUCCEEDED",
		"rows": []interface{}{
			map[string]interface{}{"value": json.Number("1")},
		},
	}, output)
}

func TestExecuteActionPreservesSanitizedBridgeErrorBody(t *testing.T) {
	client := &captureBridgeClient{
		response: []byte(`{"status":"target_not_found","error":{"code":"target_not_found","message":"no matching integration check found"}}`),
		err:      errors.New("status 404"),
	}
	action := NewExecuteAction(func() (BridgeClient, string, error) {
		return client, "https://localhost:5001" + AgentRemoteQueryExecuteEndpointPath, nil
	})

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "secret-db"},
		"query":       "SELECT 1 AS value",
	}), nil)

	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"status": "target_not_found",
		"error": map[string]interface{}{
			"code":    "target_not_found",
			"message": "no matching integration check found",
		},
	}, output)
}

func TestExecuteActionSanitizesInputExtractionErrors(t *testing.T) {
	action := NewExecuteAction(func() (BridgeClient, string, error) {
		require.Fail(t, "bridge client should not be created for invalid inputs")
		return nil, "", nil
	})

	_, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "secret-db"},
		"query":       "SELECT secret FROM private_table",
		"bad":         make(chan struct{}),
	}), nil)

	require.Error(t, err)
	var parErr util.PARError
	require.ErrorAs(t, err, &parErr)
	assert.Equal(t, "invalid remote query action inputs", parErr.Message)
	assert.Equal(t, "invalid remote query action inputs", parErr.ExternalMessage)
	assert.NotContains(t, err.Error(), "secret-db")
	assert.NotContains(t, err.Error(), "SELECT secret")
}

func taskWithInputs(inputs map[string]interface{}) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		BundleID: BundleID,
		Name:     ExecuteActionName,
		Inputs:   inputs,
	}
	return task
}

type captureBridgeClient struct {
	url         string
	contentType string
	body        string
	response    []byte
	err         error
}

func (c *captureBridgeClient) Post(url string, contentType string, body io.Reader, _ ...ipc.RequestOption) ([]byte, error) {
	c.url = url
	c.contentType = contentType
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	c.body = string(payload)
	return c.response, c.err
}
