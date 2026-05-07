// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ncm

package com_datadoghq_remoteaction_networkconfigmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// storedConfigResponse mirrors the GetConfigResponse struct returned by the agent's
// /agent/ncm/config IPC endpoint.
type storedConfigResponse struct {
	ConfigUUID string `json:"config_uuid"`
	DeviceID   string `json:"device_id"`
	ConfigType string `json:"config_type"`
	CapturedAt int64  `json:"captured_at"`
	RawConfig  string `json:"raw_config"`
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
	// ConfigUUID is the identifier of the stored config snapshot to roll back to.
	ConfigUUID string `json:"config_uuid"`
	// DeviceID identifies the device to roll back
	DeviceID string `json:"device_id"`
	// ConfigHash is the hashed value of the config; the operation will abort if
	// this doesn't match what we have in storage.
	ConfigHash string `json:"config_hash"`
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
	if inputs.ConfigUUID == "" {
		return nil, errors.New("rollbackConfig: config_uuid input is required")
	}

	endpoint, err := h.ipcClient.NewIPCEndpoint("/agent/ncm/config")
	if err != nil {
		return nil, fmt.Errorf("failed to create NCM config IPC endpoint: %w", err)
	}

	res, err := endpoint.DoGet(ipchttp.WithValues(url.Values{
		"uuid":      {inputs.ConfigUUID},
		"hash":      {inputs.ConfigHash},
		"device_id": {inputs.DeviceID},
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve stored config %s from agent: %w", inputs.ConfigUUID, err)
	}

	var storedConfig storedConfigResponse
	if err := json.Unmarshal(res, &storedConfig); err != nil {
		return nil, fmt.Errorf("failed to parse stored config response: %w", err)
	}

	ncmConf, err := config.GetNCMContextFromCoreCheck(h.ipcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve NCM config from agent: %w", err)
	}
	device, ok := ncmConf.Devices["172.17.0.2"] // TODO FIXME map device ID to IP
	if !ok {
		fmt.Println(ncmConf.Devices)
		return nil, fmt.Errorf("unrecognized device %q", inputs.DeviceID)
	}
	client, err := ncmremote.NewSSHClient(&device)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", inputs.DeviceID, err)
	}

	err = client.Connect()
	if err != nil {
		return nil, err
	}
	if err := client.PushBoth(storedConfig.RawConfig); err != nil {
		return nil, err
	}

	return nil, nil
}
