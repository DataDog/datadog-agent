// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkconfigmanagement

import (
	"context"
	"errors"
	"fmt"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// RollbackConfigHandler handles the rollbackConfig action for network config management
type RollbackConfigHandler struct {
	ipcClient ipc.HTTPClient
}

// NewRollbackConfigHandler creates a new RollbackConfigHandler
func NewRollbackConfigHandler(client ipc.HTTPClient) *RollbackConfigHandler {
	return &RollbackConfigHandler{ipcClient: client}
}

// RollbackConfigInputs defines the inputs for the rollbackConfig action
type RollbackConfigInputs struct {
	// ConfigVersion is the identifier of the stored config snapshot to roll back to.
	ConfigVersion string `json:"configVersion"`
	// DeviceID identifies the device to roll back.
	DeviceID string `json:"deviceID"`
	// ConfigHash is the hashed value of the config; the operation will abort if
	// this doesn't match what we have in storage.
	ConfigHash string `json:"hash"`
}

// RollbackConfigOutputs is the output of a rollbackConfig action.
type RollbackConfigOutputs struct {
	Success bool   `json:"success,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Run executes the rollbackConfig action
func (h *RollbackConfigHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("IPC client is not available")
	}

	inputs, err := types.ExtractInputs[RollbackConfigInputs](task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rollbackConfig inputs: %w", err)
	}
	if inputs.ConfigVersion == "" {
		return nil, errors.New("rollbackConfig: ConfigVersion input is required")
	}

	if err := executeRollback(inputs.DeviceID, inputs.ConfigHash, inputs.ConfigVersion); err != nil {
		return nil, fmt.Errorf("rollback failed: %w", err)
	}

	return RollbackConfigOutputs{
		Success: false,
		Error:   "not implemented",
	}, nil
}

func executeRollback(deviceID string, configHash string, configVersion string) error {
	_, _, _ = deviceID, configHash, configVersion
	return errors.ErrUnsupported
}
