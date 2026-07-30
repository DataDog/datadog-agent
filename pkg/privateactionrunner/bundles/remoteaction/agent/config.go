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

// GetConfigHandler handles the getConfig action, returning either the full
// resolved agent configuration or a single configuration value as JSON.
type GetConfigHandler struct {
	ipcClient ipc.HTTPClient
}

// NewGetConfigHandler creates a new GetConfigHandler.
func NewGetConfigHandler(client ipc.HTTPClient) *GetConfigHandler {
	return &GetConfigHandler{ipcClient: client}
}

// GetConfigInputs defines the inputs for the getConfig action.
type GetConfigInputs struct {
	// Key optionally selects a single configuration setting (for example
	// "log_level"). When empty the full resolved configuration is returned.
	Key string `json:"key"`
}

// Run executes the getConfig action.
func (h *GetConfigHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("getConfig: IPC client is not available")
	}

	inputs, err := types.ExtractInputs[GetConfigInputs](task)
	if err != nil {
		return nil, fmt.Errorf("getConfig: failed to parse inputs: %w", err)
	}

	base, err := agentBaseURL()
	if err != nil {
		return nil, fmt.Errorf("getConfig: %w", err)
	}

	// Always read the full resolved configuration, which the agent returns as
	// scrubbed YAML. When a single key is requested, select it from that
	// document: the per-key runtime-settings endpoint (/agent/config/{key})
	// only serves registered runtime-settable settings and returns HTTP 400 on
	// ordinary keys such as "site" or "logs_enabled", whereas selecting from the
	// full config supports any key and stays scrubbed.
	configURL := base + "/agent/config"
	resp, err := h.ipcClient.Get(configURL, ipchttp.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("getConfig: request to agent failed: %w", err)
	}
	full := decodeAgentYAML(resp)

	if inputs.Key != "" {
		value, ok := selectConfigValue(full, inputs.Key)
		if !ok {
			return nil, fmt.Errorf("getConfig: key %q not found in agent configuration", inputs.Key)
		}
		return map[string]interface{}{"key": inputs.Key, "value": value}, nil
	}
	return full, nil
}

// selectConfigValue resolves a configuration key against the full config map.
// It first tries an exact top-level match, then falls back to treating dots as
// nesting (viper-style keys such as "logs_config.container_collect_all").
func selectConfigValue(full map[string]interface{}, key string) (interface{}, bool) {
	if v, ok := full[key]; ok {
		return v, true
	}
	var cur interface{} = full
	for _, part := range strings.Split(key, ".") {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		v, ok := m[part]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}
