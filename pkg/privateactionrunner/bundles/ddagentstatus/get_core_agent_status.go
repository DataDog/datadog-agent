// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// GetCoreAgentStatusHandler retrieves the core agent's status via the IPC API.
type GetCoreAgentStatusHandler struct {
	ipcClient ipc.HTTPClient
}

// NewGetCoreAgentStatusHandler creates a new handler with the given IPC client.
func NewGetCoreAgentStatusHandler(client ipc.HTTPClient) *GetCoreAgentStatusHandler {
	return &GetCoreAgentStatusHandler{
		ipcClient: client,
	}
}

// GetCoreAgentStatusInputs defines the optional inputs for the getCoreAgentStatus action.
type GetCoreAgentStatusInputs struct {
	Verbose bool   `json:"verbose,omitempty"`
	Section string `json:"section,omitempty"`
}

// Run executes the getCoreAgentStatus action by calling the agent's IPC status endpoint.
func (h *GetCoreAgentStatusHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("IPC client is not available")
	}

	inputs, err := types.ExtractInputs[GetCoreAgentStatusInputs](task)
	if err != nil {
		return nil, fmt.Errorf("failed to extract inputs: %w", err)
	}

	endpointPath := "/agent/status"
	if inputs.Section != "" {
		endpointPath = fmt.Sprintf("/agent/%s/status", inputs.Section)
	}

	endpoint, err := h.ipcClient.NewIPCEndpoint(endpointPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPC endpoint: %w", err)
	}

	values := url.Values{}
	values.Set("format", "json")
	if inputs.Verbose {
		values.Set("verbose", "true")
	}

	body, err := endpoint.DoGet(ipchttp.WithValues(values), ipchttp.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to get agent status: %w", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent status response: %w", err)
	}

	return status, nil
}
