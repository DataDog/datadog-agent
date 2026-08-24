// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_datadogagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// GetDogstatsdTopHandler returns the metrics with the most active DogStatsD contexts.
type GetDogstatsdTopHandler struct {
	ipcClient ipc.HTTPClient
}

// NewGetDogstatsdTopHandler creates a new GetDogstatsdTopHandler.
func NewGetDogstatsdTopHandler(client ipc.HTTPClient) *GetDogstatsdTopHandler {
	return &GetDogstatsdTopHandler{ipcClient: client}
}

// GetDogstatsdTopInputs defines the display limits for the DogStatsD context summary.
type GetDogstatsdTopInputs struct {
	NumMetrics int `json:"num_metrics,omitempty"`
	NumTags    int `json:"num_tags,omitempty"`
}

// Run executes the getDogstatsdTop action.
func (h *GetDogstatsdTopHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("getDogstatsdTop: IPC client is not available")
	}

	inputs, err := types.ExtractInputs[GetDogstatsdTopInputs](task)
	if err != nil {
		return nil, fmt.Errorf("getDogstatsdTop: failed to parse inputs: %w", err)
	}
	body, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("getDogstatsdTop: failed to encode request: %w", err)
	}

	base, err := agentBaseURL()
	if err != nil {
		return nil, fmt.Errorf("getDogstatsdTop: %w", err)
	}
	endpointURL := base + "/agent/dogstatsd-contexts-top"

	resp, err := h.ipcClient.Post(endpointURL, "application/json", bytes.NewReader(body), ipchttp.WithContext(ctx))
	if err != nil {
		msg := strings.TrimSpace(string(resp))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("getDogstatsdTop: request to agent failed: %s", msg)
	}

	return decodeAgentObject(resp), nil
}
