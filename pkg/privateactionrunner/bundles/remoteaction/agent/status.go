// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_agent

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// GetStatusHandler handles the getStatus action, returning the local agent's
// status as JSON.
type GetStatusHandler struct {
	ipcClient ipc.HTTPClient
}

// NewGetStatusHandler creates a new GetStatusHandler.
func NewGetStatusHandler(client ipc.HTTPClient) *GetStatusHandler {
	return &GetStatusHandler{ipcClient: client}
}

// GetStatusInputs defines the inputs for the getStatus action.
type GetStatusInputs struct {
	// Section optionally restricts the status to a single named section (for
	// example "collector"). When empty the full status is returned.
	Section string `json:"section"`
}

// Run executes the getStatus action.
func (h *GetStatusHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("getStatus: IPC client is not available")
	}

	inputs, err := types.ExtractInputs[GetStatusInputs](task)
	if err != nil {
		return nil, fmt.Errorf("getStatus: failed to parse inputs: %w", err)
	}

	base, err := agentBaseURL()
	if err != nil {
		return nil, fmt.Errorf("getStatus: %w", err)
	}

	path := "/agent/status"
	if inputs.Section != "" {
		path = "/agent/status/section/" + url.PathEscape(inputs.Section)
	}
	statusURL := base + path + "?format=json"

	resp, err := h.ipcClient.Get(statusURL, ipchttp.WithContext(ctx))
	if err != nil {
		if msg := strings.TrimSpace(string(resp)); msg != "" {
			return nil, fmt.Errorf("getStatus: request to agent failed: %s", msg)
		}
		return nil, fmt.Errorf("getStatus: request to agent failed: %w", err)
	}

	return decodeAgentObject(resp), nil
}
