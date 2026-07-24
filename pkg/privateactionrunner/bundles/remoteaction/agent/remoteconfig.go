// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// GetRemoteConfigStateHandler handles the getRemoteConfigState action, returning
// the state of the agent's remote configuration repositories as JSON.
type GetRemoteConfigStateHandler struct {
	ipcClient ipc.HTTPClient
}

// NewGetRemoteConfigStateHandler creates a new GetRemoteConfigStateHandler.
func NewGetRemoteConfigStateHandler(client ipc.HTTPClient) *GetRemoteConfigStateHandler {
	return &GetRemoteConfigStateHandler{ipcClient: client}
}

// Run executes the getRemoteConfigState action. It takes no inputs.
func (h *GetRemoteConfigStateHandler) Run(
	ctx context.Context,
	_ *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("getRemoteConfigState: IPC client is not available")
	}

	base, err := agentBaseURL()
	if err != nil {
		return nil, fmt.Errorf("getRemoteConfigState: %w", err)
	}
	stateURL := base + "/agent/remote-config/state"

	resp, err := h.ipcClient.Get(stateURL, ipchttp.WithContext(ctx))
	if err != nil {
		msg := strings.TrimSpace(string(resp))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("getRemoteConfigState: request to agent failed: %s", msg)
	}

	return decodeAgentObject(resp), nil
}
