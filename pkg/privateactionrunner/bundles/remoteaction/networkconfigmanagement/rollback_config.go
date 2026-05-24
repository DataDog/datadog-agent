// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkconfigmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	ncmtypes "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// storedConfigResponse mirrors the GetConfigResponse struct returned by the agent's
// /agent/ncm/config IPC endpoint.
type storedConfigResponse struct {
	ConfigUUID string              `json:"config_uuid"`
	DeviceID   string              `json:"device_id"`
	ConfigType ncmtypes.ConfigType `json:"config_type"`
	CapturedAt int64               `json:"captured_at"`
	RawConfig  string              `json:"raw_config"`
}

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

	endpoint, err := h.ipcClient.NewIPCEndpoint("/agent/ncm/config")
	if err != nil {
		return nil, fmt.Errorf("failed to create NCM config IPC endpoint: %w", err)
	}

	res, err := endpoint.DoGet(ipchttp.WithContext(ctx), ipchttp.WithValues(url.Values{
		"uuid": {inputs.ConfigVersion},
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve stored config %q from agent: %w", inputs.ConfigVersion, err)
	}

	var storedConfig storedConfigResponse
	if err := json.Unmarshal(res, &storedConfig); err != nil {
		return nil, fmt.Errorf("failed to parse stored config response: %w", err)
	}

	if storedConfig.DeviceID != inputs.DeviceID {
		return nil, fmt.Errorf("input mismatch: config %q is not for device %q", inputs.ConfigVersion, inputs.DeviceID)
	}

	expectedHash := ncmstore.HashConfig(storedConfig.RawConfig)
	if expectedHash != inputs.ConfigHash {
		return nil, fmt.Errorf("hash mismatch for config %q", inputs.ConfigVersion)
	}

	ncmConf, err := ncmconfig.GetNCMContextFromCoreCheck(ctx, h.ipcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve NCM config from agent: %w", err)
	}
	_, ipAddress, ok := strings.Cut(inputs.DeviceID, ":")
	if !ok {
		return nil, fmt.Errorf("malformed device ID %q: expected namespace:ip_address", inputs.DeviceID)
	}

	device, ok := ncmConf.Devices[ipAddress]
	if !ok {
		return nil, fmt.Errorf("no NCM configuration for device: %q", inputs.DeviceID)
	}

	client, err := ncmremote.NewSSHConnector(&device)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", inputs.DeviceID, err)
	}
	// TODO set profile properly
	conn, err := client.Connect()
	if err != nil {
		return nil, fmt.Errorf("%v: %w", inputs.DeviceID, err)
	}
	defer conn.Close()

	err = conn.PushConfig(ctx, storedConfig.RawConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot push config to device %q: %w", inputs.DeviceID, err)
	}

	return RollbackConfigOutputs{
		Success: false,
		Error:   "not implemented",
	}, nil
}
