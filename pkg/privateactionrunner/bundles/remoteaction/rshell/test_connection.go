// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// TestConnectionHandler implements the testConnection action.
type TestConnectionHandler struct{}

// NewTestConnectionHandler creates a new TestConnectionHandler.
func NewTestConnectionHandler() *TestConnectionHandler {
	return &TestConnectionHandler{}
}

// TestConnectionOutputs defines the outputs for the testConnection action.
type TestConnectionOutputs struct {
	Success bool   `json:"success"`
	Version string `json:"version"`
}

// Run returns success and the runner version to confirm the rshell bundle is reachable.
func (h *TestConnectionHandler) Run(
	ctx context.Context,
	_ *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return &TestConnectionOutputs{
		Success: true,
		Version: parversion.RunnerVersion,
	}, nil
}
