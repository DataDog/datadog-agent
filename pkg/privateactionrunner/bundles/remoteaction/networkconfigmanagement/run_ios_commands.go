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

// RunIOSCommandsHandler handles the runIOSCommands action for network config management
type RunIOSCommandsHandler struct {
	ipcClient ipc.HTTPClient
	clock     clock.Clock
}

// NewRunIOSCommandsHandler creates a new RunIOSCommandsHandler
func NewRunIOSCommandsHandler(client ipc.HTTPClient) *RunIOSCommandsHandler {
	return &RunIOSCommandsHandler{
		ipcClient: client,
		clock:     clock.New(),
	}
}

// RunIOSCommandsInputs defines the inputs for the runIOSCommands action
type RunIOSCommandsInputs struct {
	// DeviceID identifies the device to run the command on.
	DeviceID string `json:"deviceID"`
	// Command is a JSON-encoded ioscmd.CommandBlock. It is passed through to
	// the agent unchanged; the agent parses and validates it.
	Command json.RawMessage `json:"command"`
	// CredentialSet selects which credential set to use when connecting to the device.
	CredentialSet string `json:"credentialSet"`
}

// RunIOSCommandsOutputs is the output of a runIOSCommands action.
type RunIOSCommandsOutputs struct {
	Success       bool                    `json:"success,omitempty"`
	CommandResult *ncmtypes.CommandResult `json:"command_result"`
	Error         string                  `json:"error,omitempty"`
	ErrorCode     string                  `json:"error_code,omitempty"`
	FinishedAt    *time.Time              `json:"finished_at,omitempty"`
}

// Run executes the runIOSCommands action
func (h *RunIOSCommandsHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("IPC client is not available")
	}

	inputs, err := types.ExtractInputs[RunIOSCommandsInputs](task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse runIOSCommands inputs: %w", err)
	}
	if len(inputs.Command) == 0 || string(inputs.Command) == "null" {
		return nil, errors.New("runIOSCommands: Command input is required")
	}

	body, err := json.Marshal(map[string]any{
		"device_id":      inputs.DeviceID,
		"command":        inputs.Command,
		"credential_set": inputs.CredentialSet,
	})
	if err != nil {
		return nil, fmt.Errorf("runIOSCommands: failed to marshal request: %w", err)
	}

	ipcAddress, err := pkgconfighelper.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("runIOSCommands: failed to get IPC address: %w", err)
	}
	port := pkgconfigsetup.Datadog().GetInt("cmd_port")
	url := fmt.Sprintf("https://%s/agent/ncm/run-ios-commands", net.JoinHostPort(ipcAddress, strconv.Itoa(port)))

	resp, err := h.ipcClient.Post(url, "application/json", bytes.NewBuffer(body), ipchttp.WithContext(ctx))
	if err != nil {
		// This case only happens when there's an internal error - errors during
		// the command execution itself are returned in the RunIOSCommandsResponse.
		// The response here should be a struct like `{"error":"<error message>"}`
		errMsg := strings.TrimSpace(string(resp))
		if errMsg == "" {
			errMsg = err.Error()
		}
		return RunIOSCommandsOutputs{Error: errMsg}, err
	}
	var response *ncmtypes.RunIOSCommandsResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return RunIOSCommandsOutputs{Error: err.Error()}, fmt.Errorf("unable to unmarshal run-ios-commands response: %w", err)
	}
	t := h.clock.Now()
	var result RunIOSCommandsOutputs
	result.Success = response.ErrorCode == ""
	result.FinishedAt = &t
	result.Error = response.ErrorMsg
	result.ErrorCode = response.ErrorCode
	result.CommandResult = response.CommandResult
	return result, nil
}
