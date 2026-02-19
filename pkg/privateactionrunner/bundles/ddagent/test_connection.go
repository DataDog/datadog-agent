// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type TestConnectionHandler struct {
}

func NewTestConnectionHandler() *TestConnectionHandler {
	return &TestConnectionHandler{}
}

type TestConnectionInputs struct {
	// No inputs required for this test action
}

type TestConnectionOutputs struct {
	Success   bool      `json:"success"`
	AgentInfo AgentInfo `json:"version"`
}

type AgentInfo struct {
	Version string `json:"version"`
}

func (h *TestConnectionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return &TestConnectionOutputs{
		Success: true,
		AgentInfo: AgentInfo{
			Version: parversion.RunnerVersion,
		},
	}, nil
}
