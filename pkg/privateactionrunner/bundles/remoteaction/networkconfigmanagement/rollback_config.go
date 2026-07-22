// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkconfigmanagement

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/benbjohnson/clock"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	networkconfigmanagementimpl "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/impl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// RollbackConfigHandler handles the rollbackConfig action for network config management
type RollbackConfigHandler struct {
	ipcClient ipc.HTTPClient
	clock     clock.Clock
}

// NewRollbackConfigHandler creates a new RollbackConfigHandler
func NewRollbackConfigHandler(client ipc.HTTPClient) *RollbackConfigHandler {
	return &RollbackConfigHandler{
		ipcClient: client,
		clock:     clock.New(),
	}
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
	Success        bool `json:"success,omitempty"`
	CommandResults *ncmremote.PushResult
	Error          string `json:"error,omitempty"`
	ErrorCode      string
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
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

	body, err := json.Marshal(map[string]string{
		"device_id":      inputs.DeviceID,
		"config_version": inputs.ConfigVersion,
		"hash":           inputs.ConfigHash,
	})
	if err != nil {
		return nil, fmt.Errorf("rollbackConfig: failed to marshal request: %w", err)
	}

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("rollbackConfig: failed to get IPC address: %w", err)
	}
	port := pkgconfigsetup.Datadog().GetInt("cmd_port")
	url := fmt.Sprintf("https://%s/agent/ncm/rollback", net.JoinHostPort(ipcAddress, strconv.Itoa(port)))

	resp, err := h.ipcClient.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		// This case only happens when there's an internal error - errors during
		// the rollback itself are returned in the RollbackResult. The response
		// here should be a struct like `{"error":"<error message>"}`
		errMsg := strings.TrimSpace(string(resp))
		if errMsg == "" {
			errMsg = err.Error()
		}
		return RollbackConfigOutputs{Error: errMsg}, err
	}
	var response *networkconfigmanagementimpl.RollbackResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return RollbackConfigOutputs{Error: err.Error()}, fmt.Errorf("unable to unmarshal rollback response: %w", err)
	}
	t := h.clock.Now()
	var result RollbackConfigOutputs
	result.Success = response.ErrorCode == ""
	result.FinishedAt = &t
	result.Error = response.ErrorMsg
	result.ErrorCode = response.ErrorCode
	result.CommandResults = response.CommandResults
	return result, nil
}
