// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// GetDiagnoseHandler handles the getDiagnose action, running the agent's
// in-process diagnose suites and returning the result as JSON.
type GetDiagnoseHandler struct {
	ipcClient ipc.HTTPClient
}

// NewGetDiagnoseHandler creates a new GetDiagnoseHandler.
func NewGetDiagnoseHandler(client ipc.HTTPClient) *GetDiagnoseHandler {
	return &GetDiagnoseHandler{ipcClient: client}
}

// GetDiagnoseInputs defines the inputs for the getDiagnose action.
type GetDiagnoseInputs struct {
	// Verbose enables verbose diagnosis output.
	Verbose bool `json:"verbose"`
	// Include optionally restricts the run to the named suites. When empty all
	// suites run. Unknown suite names are rejected by the agent.
	Include []string `json:"include"`
	// Exclude optionally skips the named suites.
	Exclude []string `json:"exclude"`
}

// Run executes the getDiagnose action.
func (h *GetDiagnoseHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("getDiagnose: IPC client is not available")
	}

	inputs, err := types.ExtractInputs[GetDiagnoseInputs](task)
	if err != nil {
		return nil, fmt.Errorf("getDiagnose: failed to parse inputs: %w", err)
	}

	cfg := diagnose.Config{
		Verbose: inputs.Verbose,
		Include: inputs.Include,
		Exclude: inputs.Exclude,
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("getDiagnose: failed to encode diagnose config: %w", err)
	}

	base, err := agentBaseURL()
	if err != nil {
		return nil, fmt.Errorf("getDiagnose: %w", err)
	}
	diagnoseURL := base + "/agent/diagnose"

	resp, err := h.ipcClient.Post(diagnoseURL, "application/json", bytes.NewBuffer(body), ipchttp.WithContext(ctx))
	if err != nil {
		msg := strings.TrimSpace(string(resp))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("getDiagnose: request to agent failed: %s", msg)
	}

	return decodeAgentObject(resp), nil
}
