// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remotequeries

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteActionUsesCredentialFreeAgentSecureRequestShape(t *testing.T) {
	row, err := structpb.NewStruct(map[string]interface{}{"value": 1})
	require.NoError(t, err)
	client := &captureBridgeClient{response: &pb.RemoteQueryExecuteResponse{Status: "SUCCEEDED", Rows: []*structpb.Struct{row}}}
	action := NewExecuteAction(func() (BridgeClient, error) {
		return client, nil
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
	require.NotNil(t, client.request)
	assert.Equal(t, "postgres", client.request.GetIntegration())
	assert.Equal(t, "localhost", client.request.GetTarget().GetHost())
	assert.Equal(t, int32(5432), client.request.GetTarget().GetPort())
	assert.Equal(t, "postgres", client.request.GetTarget().GetDbname())
	assert.Equal(t, "SELECT 1 AS value", client.request.GetQuery())
	assert.Equal(t, int32(1), client.request.GetLimits().GetMaxRows())
	requestEvidence, err := json.Marshal(client.request)
	require.NoError(t, err)
	assert.NotContains(t, string(requestEvidence), "secret-value")
	assert.Equal(t, map[string]interface{}{
		"status": "SUCCEEDED",
		"rows": []interface{}{
			map[string]interface{}{"value": float64(1)},
		},
	}, output)
}

func TestExecuteActionPreservesSanitizedBridgeErrorBody(t *testing.T) {
	client := &captureBridgeClient{
		response: &pb.RemoteQueryExecuteResponse{Status: "target_not_found", Error: &pb.RemoteQueryExecuteError{Code: "target_not_found", Message: "no matching integration check found"}},
	}
	action := NewExecuteAction(func() (BridgeClient, error) {
		return client, nil
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
	action := NewExecuteAction(func() (BridgeClient, error) {
		require.Fail(t, "bridge client should not be created for invalid inputs")
		return nil, nil
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
	request  *pb.RemoteQueryExecuteRequest
	response *pb.RemoteQueryExecuteResponse
	err      error
}

func (c *captureBridgeClient) RemoteQueryExecute(_ context.Context, req *pb.RemoteQueryExecuteRequest, _ ...grpc.CallOption) (*pb.RemoteQueryExecuteResponse, error) {
	c.request = req
	return c.response, c.err
}
