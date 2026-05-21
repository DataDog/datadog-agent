// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remotequeries

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// BridgeClient is the narrow AgentSecure gRPC client surface required by this action.
type BridgeClient interface {
	RemoteQueryExecute(ctx context.Context, in *pb.RemoteQueryExecuteRequest, opts ...grpc.CallOption) (*pb.RemoteQueryExecuteResponse, error)
}

// BridgeClientFactory returns an authenticated AgentSecure client over the local Agent IPC channel.
type BridgeClientFactory func() (BridgeClient, error)

type ExecuteAction struct {
	newBridgeClient BridgeClientFactory
}

func NewExecuteAction(newBridgeClient BridgeClientFactory) *ExecuteAction {
	return &ExecuteAction{newBridgeClient: newBridgeClient}
}

type ExecuteInputs struct {
	Integration string        `json:"integration"`
	Target      TargetInputs  `json:"target"`
	Query       string        `json:"query"`
	Limits      *LimitsInputs `json:"limits,omitempty"`
}

type TargetInputs struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	DBName string `json:"dbname"`
}

type LimitsInputs struct {
	MaxRows   int `json:"maxRows"`
	MaxBytes  int `json:"maxBytes"`
	TimeoutMs int `json:"timeoutMs"`
}

func (a *ExecuteAction) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[ExecuteInputs](task)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(
			fmt.Errorf("invalid remote query action inputs"),
			"invalid remote query action inputs",
		)
	}

	if a == nil || a.newBridgeClient == nil {
		return nil, util.DefaultActionError(fmt.Errorf("remote query action requires an Agent IPC client"))
	}
	client, err := a.newBridgeClient()
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query action could not create an Agent IPC client")
	}
	if client == nil {
		return nil, util.DefaultActionError(fmt.Errorf("remote query action requires an AgentSecure client"))
	}

	resp, err := client.RemoteQueryExecute(ctx, remoteQueryExecuteRequestFromInputs(inputs))
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure RPC failed")
	}
	output, err := remoteQueryExecuteOutputFromProto(resp)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure RPC response was invalid")
	}
	return output, nil
}

func remoteQueryExecuteRequestFromInputs(inputs ExecuteInputs) *pb.RemoteQueryExecuteRequest {
	req := &pb.RemoteQueryExecuteRequest{
		Integration: inputs.Integration,
		Target: &pb.RemoteQueryTarget{
			Host:   inputs.Target.Host,
			Port:   int32(inputs.Target.Port),
			Dbname: inputs.Target.DBName,
		},
		Query: inputs.Query,
	}
	if inputs.Limits != nil {
		req.Limits = &pb.RemoteQueryExecuteLimits{
			MaxRows:   int32(inputs.Limits.MaxRows),
			MaxBytes:  int32(inputs.Limits.MaxBytes),
			TimeoutMs: int32(inputs.Limits.TimeoutMs),
		}
	}
	return req
}

func remoteQueryExecuteOutputFromProto(resp *pb.RemoteQueryExecuteResponse) (map[string]interface{}, error) {
	if resp == nil || resp.GetStatus() == "" {
		return nil, fmt.Errorf("remote query response missing status")
	}

	output := map[string]interface{}{"status": resp.GetStatus()}
	if resp.GetError() != nil {
		output["error"] = map[string]interface{}{
			"code":    resp.GetError().GetCode(),
			"message": resp.GetError().GetMessage(),
		}
	}
	if len(resp.GetColumns()) > 0 {
		columns := make([]interface{}, 0, len(resp.GetColumns()))
		for _, column := range resp.GetColumns() {
			columns = append(columns, column.AsMap())
		}
		output["columns"] = columns
	}
	if len(resp.GetRows()) > 0 {
		rows := make([]interface{}, 0, len(resp.GetRows()))
		for _, row := range resp.GetRows() {
			rows = append(rows, row.AsMap())
		}
		output["rows"] = rows
	}
	if resp.GetTruncated() {
		output["truncated"] = true
	}
	if resp.GetStats() != nil {
		output["stats"] = resp.GetStats().AsMap()
	}
	return output, nil
}
