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
	"strconv"
	"strings"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// GenerateFlareHandler handles the generateFlare action, asking the local agent
// to build a flare archive on the agent host.
//
// This creates the archive on the agent host's filesystem and returns its path;
// it does not upload the archive to a Datadog support case. Uploading to a case
// is a planned follow-up (it requires either an agent-side send endpoint or
// upload support in the runner).
type GenerateFlareHandler struct {
	ipcClient ipc.HTTPClient
}

// NewGenerateFlareHandler creates a new GenerateFlareHandler.
func NewGenerateFlareHandler(client ipc.HTTPClient) *GenerateFlareHandler {
	return &GenerateFlareHandler{ipcClient: client}
}

// GenerateFlareInputs defines the inputs for the generateFlare action.
type GenerateFlareInputs struct {
	// ProviderTimeoutSeconds optionally bounds how long each flare provider may
	// run. When zero the agent default is used.
	ProviderTimeoutSeconds int `json:"providerTimeoutSeconds"`
}

// Run executes the generateFlare action.
func (h *GenerateFlareHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("generateFlare: IPC client is not available")
	}

	inputs, err := types.ExtractInputs[GenerateFlareInputs](task)
	if err != nil {
		return nil, fmt.Errorf("generateFlare: failed to parse inputs: %w", err)
	}

	base, err := agentBaseURL()
	if err != nil {
		return nil, fmt.Errorf("generateFlare: %w", err)
	}
	flareURL := base + "/agent/flare"
	if inputs.ProviderTimeoutSeconds > 0 {
		// The endpoint expects the provider timeout as a duration in nanoseconds.
		timeoutNanos := int64(inputs.ProviderTimeoutSeconds) * int64(1_000_000_000)
		q := url.Values{"provider_timeout": {strconv.FormatInt(timeoutNanos, 10)}}
		flareURL += "?" + q.Encode()
	}

	// The endpoint takes the (optional) profile data as its JSON body; an empty
	// object requests a flare with no profiling data attached.
	resp, err := h.ipcClient.Post(flareURL, "application/json", strings.NewReader("{}"), ipchttp.WithContext(ctx))
	if err != nil {
		msg := strings.TrimSpace(string(resp))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("generateFlare: request to agent failed: %s", msg)
	}

	return map[string]interface{}{
		"archivePath": strings.TrimSpace(string(resp)),
		"note":        "Flare archive was created on the agent host. It has not been uploaded to a Datadog support case.",
	}, nil
}
