// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkdevices

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/discovery"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// TriggerDiscoveryRequest is the action's input.
type TriggerDiscoveryRequest struct {
	AutodiscoveryID string `json:"autodiscoveryId"`
}

// TriggerDiscoveryResponse is the action's output. A refusal by the Agent is a
// result rather than a task failure, so the reason reaches the caller.
type TriggerDiscoveryResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// TriggerDiscoveryHandler starts a discovery run on request.
type TriggerDiscoveryHandler struct {
	ipcClient ipc.HTTPClient
}

// NewTriggerDiscoveryHandler builds the triggerNdmDiscovery action.
func NewTriggerDiscoveryHandler(ipcClient ipc.HTTPClient) *TriggerDiscoveryHandler {
	return &TriggerDiscoveryHandler{ipcClient: ipcClient}
}

// Run asks the Agent to sweep one range now.
func (h *TriggerDiscoveryHandler) Run(ctx context.Context, task *types.Task, _ *privateconnection.PrivateCredentials) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("IPC client is not available")
	}

	req, err := types.ExtractInputs[TriggerDiscoveryRequest](task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse triggerNdmDiscovery inputs: %w", err)
	}

	refusal, err := h.trigger(ctx, req)
	if err != nil {
		return nil, err
	}
	if refusal != "" {
		return &TriggerDiscoveryResponse{Success: false, Error: refusal}, nil
	}
	return &TriggerDiscoveryResponse{Success: true}, nil
}

// trigger posts the request and returns the Agent's refusal, if it refused.
// An empty reason with a nil error means the sweep was queued.
func (h *TriggerDiscoveryHandler) trigger(ctx context.Context, req TriggerDiscoveryRequest) (string, error) {
	body, err := json.Marshal(discovery.TriggerRequest{AutodiscoveryID: req.AutodiscoveryID})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url, err := triggerDiscoveryURL()
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	respBody, err := h.ipcClient.Do(httpReq)
	if err == nil {
		return "", nil
	}

	// The Agent answers a refusal with a JSON error body. Anything else means
	// it could not be reached, which is a task failure rather than a result.
	if reason := refusalReason(respBody); reason != "" {
		return reason, nil
	}
	return "", fmt.Errorf("failed to reach the Agent: %w", err)
}

// refusalReason reads the Agent's error body, and is empty for anything else.
func refusalReason(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return payload.Error
}

func triggerDiscoveryURL() (string, error) {
	ipcAddress, err := pkgconfighelper.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return "", fmt.Errorf("failed to get IPC address: %w", err)
	}

	port := pkgconfigsetup.Datadog().GetInt("cmd_port")
	return fmt.Sprintf("https://%s/agent/networkdevices/trigger-discovery", net.JoinHostPort(ipcAddress, strconv.Itoa(port))), nil
}
