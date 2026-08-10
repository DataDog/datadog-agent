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
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	ncmtypes "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// RunCommandHandler handles the runCommand action for network config management
type RunCommandHandler struct {
	ipcClient ipc.HTTPClient
	clock     clock.Clock
}

// NewRunCommandHandler creates a new RunCommandHandler
func NewRunCommandHandler(client ipc.HTTPClient) *RunCommandHandler {
	return &RunCommandHandler{
		ipcClient: client,
		clock:     clock.New(),
	}
}

// RunCommandInputs defines the inputs for the runCommand action
type RunCommandInputs struct {
	// DeviceID identifies the device to run the command on.
	DeviceID string `json:"deviceID"`
	// Command is the command string to send to the device.
	Command string `json:"command"`
}

// RunCommandOutputs is the output of a runCommand action.
type RunCommandOutputs struct {
	Success       bool                    `json:"success,omitempty"`
	CommandResult *ncmtypes.CommandResult `json:"command_result"`
	Error         string                  `json:"error,omitempty"`
	ErrorCode     string                  `json:"error_code,omitempty"`
	FinishedAt    *time.Time              `json:"finished_at,omitempty"`
}

// Run executes the runCommand action
func (h *RunCommandHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("IPC client is not available")
	}

	inputs, err := types.ExtractInputs[RunCommandInputs](task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse runCommand inputs: %w", err)
	}
	if inputs.Command == "" {
		return nil, errors.New("runCommand: Command input is required")
	}

	body, err := json.Marshal(map[string]string{
		"device_id": inputs.DeviceID,
		"command":   inputs.Command,
	})
	if err != nil {
		return nil, fmt.Errorf("runCommand: failed to marshal request: %w", err)
	}

	ipcAddress, err := pkgconfighelper.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("runCommand: failed to get IPC address: %w", err)
	}
	port := pkgconfigsetup.Datadog().GetInt("cmd_port")
	url := fmt.Sprintf("https://%s/agent/ncm/run-command", net.JoinHostPort(ipcAddress, strconv.Itoa(port)))

	resp, err := h.ipcClient.Post(url, "application/json", bytes.NewBuffer(body), ipchttp.WithContext(ctx))
	if err != nil {
		// This case only happens when there's an internal error - errors during
		// the command execution itself are returned in the RunCommandResponse.
		// The response here should be a struct like `{"error":"<error message>"}`
		errMsg := strings.TrimSpace(string(resp))
		if errMsg == "" {
			errMsg = err.Error()
		}
		return RunCommandOutputs{Error: errMsg}, err
	}
	var response *ncmtypes.RunCommandResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return RunCommandOutputs{Error: err.Error()}, fmt.Errorf("unable to unmarshal run-command response: %w", err)
	}
	t := h.clock.Now()
	var result RunCommandOutputs
	result.Success = response.ErrorCode == ""
	result.FinishedAt = &t
	result.Error = response.ErrorMsg
	result.ErrorCode = response.ErrorCode
	result.CommandResult = response.CommandResult
	return result, nil
}
