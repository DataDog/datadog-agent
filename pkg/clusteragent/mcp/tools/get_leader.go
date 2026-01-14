// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package tools implements MCP tools for the Cluster Agent.
package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
)

// LeaderOutput contains information about the current leader
type LeaderOutput struct {
	IsLeader          bool   `json:"is_leader" jsonschema:"Whether this instance is the leader"`
	LeaderName        string `json:"leader_name" jsonschema:"Name/identity of the current leader"`
	LeaderIP          string `json:"leader_ip,omitempty" jsonschema:"IP address of the leader (empty if this instance is leader)"`
	AcquiredTime      string `json:"acquired_time,omitempty" jsonschema:"When the leadership was acquired"`
	RenewedTime       string `json:"renewed_time,omitempty" jsonschema:"When the leadership was last renewed"`
	LeaderTransitions int    `json:"leader_transitions,omitempty" jsonschema:"Number of leadership changes"`
	Error             string `json:"error,omitempty" jsonschema:"Error message if leader election information is unavailable"`
}

// GetLeader returns information about the current Cluster Agent leader
func GetLeader(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (
	*mcp.CallToolResult,
	LeaderOutput,
	error,
) {
	output := LeaderOutput{}

	// Get the leader election engine
	engine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		output.Error = fmt.Sprintf("Leader election not available: %v", err)
		return nil, output, nil
	}

	// Check if this instance is the leader
	output.IsLeader = engine.IsLeader()

	// Get the leader IP
	leaderIP, err := engine.GetLeaderIP()
	if err != nil {
		output.Error = fmt.Sprintf("Failed to get leader IP: %v", err)
		return nil, output, nil
	}
	output.LeaderIP = leaderIP

	// Get detailed leader election record
	record, err := leaderelection.GetLeaderElectionRecord()
	if err != nil {
		output.Error = fmt.Sprintf("Failed to get leader election record: %v", err)
		return nil, output, nil
	}

	output.LeaderName = record.HolderIdentity
	output.AcquiredTime = record.AcquireTime.Format(time.RFC3339)
	output.RenewedTime = record.RenewTime.Format(time.RFC3339)
	output.LeaderTransitions = int(record.LeaderTransitions)

	return nil, output, nil
}
