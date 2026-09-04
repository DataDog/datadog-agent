// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_datadogagent

import (
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

// DumpDogstatsdContextsHandler writes the local Agent's DogStatsD contexts to disk.
type DumpDogstatsdContextsHandler struct {
	ipcClient ipc.HTTPClient
}

// NewDumpDogstatsdContextsHandler creates a new DumpDogstatsdContextsHandler.
func NewDumpDogstatsdContextsHandler(client ipc.HTTPClient) *DumpDogstatsdContextsHandler {
	return &DumpDogstatsdContextsHandler{ipcClient: client}
}

// Run executes the dumpDogstatsdContexts action.
func (h *DumpDogstatsdContextsHandler) Run(
	ctx context.Context,
	_ *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("dumpDogstatsdContexts: IPC client is not available")
	}

	base, err := agentBaseURL()
	if err != nil {
		return nil, fmt.Errorf("dumpDogstatsdContexts: %w", err)
	}

	resp, err := h.ipcClient.Post(
		base+"/agent/dogstatsd-contexts-dump",
		"application/json",
		nil,
		ipchttp.WithContext(ctx),
	)
	if err != nil {
		msg := strings.TrimSpace(string(resp))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("dumpDogstatsdContexts: request to agent failed: %s", msg)
	}

	var filePath string
	if err := json.Unmarshal(resp, &filePath); err != nil {
		return nil, fmt.Errorf("dumpDogstatsdContexts: invalid response from agent: %w", err)
	}
	if strings.TrimSpace(filePath) == "" {
		return nil, errors.New("dumpDogstatsdContexts: agent returned an empty path")
	}

	return map[string]interface{}{"path": filePath}, nil
}
