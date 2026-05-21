// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remotequeries

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

const (
	// AgentRemoteQueryExecuteEndpointPath is the POC-only Agent command API path this PAR action calls.
	// It is intentionally local-only and is not a production API/IPC commitment.
	AgentRemoteQueryExecuteEndpointPath = "/agent/remote-queries/execute"
)

// BridgeClient is the narrow Agent IPC HTTP client surface required by this action.
type BridgeClient interface {
	Post(url string, contentType string, body io.Reader, opts ...ipc.RequestOption) (resp []byte, err error)
}

// BridgeClientFactory returns an IPC client and fully-qualified local Agent endpoint URL.
type BridgeClientFactory func() (BridgeClient, string, error)

type ExecuteAction struct {
	newBridgeClient BridgeClientFactory
}

func NewExecuteAction(newBridgeClient BridgeClientFactory) *ExecuteAction {
	return &ExecuteAction{newBridgeClient: newBridgeClient}
}

type ExecuteInputs struct {
	Integration string        `json:"integration"`
	Target      TargetInputs  `json:"target"`
	Query       string        `json:"query"`
	Limits      *LimitsInputs `json:"limits,omitempty"`
}

type TargetInputs struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	DBName string `json:"dbname"`
}

type LimitsInputs struct {
	MaxRows   int `json:"maxRows"`
	MaxBytes  int `json:"maxBytes"`
	TimeoutMs int `json:"timeoutMs"`
}

func (a *ExecuteAction) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[ExecuteInputs](task)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(
			fmt.Errorf("invalid remote query action inputs"),
			"invalid remote query action inputs",
		)
	}

	payload, err := json.Marshal(inputs)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(
			fmt.Errorf("marshal remote query action inputs"),
			"invalid remote query action inputs",
		)
	}

	if a == nil || a.newBridgeClient == nil {
		return nil, util.DefaultActionError(fmt.Errorf("remote query action requires an Agent IPC client"))
	}
	client, endpointURL, err := a.newBridgeClient()
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query action could not create an Agent IPC client")
	}
	if client == nil || endpointURL == "" {
		return nil, util.DefaultActionError(fmt.Errorf("remote query action requires an Agent IPC client and endpoint URL"))
	}

	body, postErr := client.Post(endpointURL, "application/json", bytes.NewReader(payload), ipchttp.WithContext(ctx))
	output, decodeErr := decodeBridgeResponse(body)
	if decodeErr == nil {
		// IPC HTTPClient returns both the response body and an error for HTTP >= 400.
		// The bridge body is already sanitized, so preserve its status/error payload as the action output.
		return output, nil
	}
	if postErr != nil {
		if len(body) > 0 {
			return nil, util.DefaultActionErrorWithDisplayError(
				fmt.Errorf("remote query IPC request failed with undecodable response"),
				"remote query IPC request failed with undecodable response",
			)
		}
		return nil, util.DefaultActionErrorWithDisplayError(postErr, "remote query IPC request failed")
	}
	return nil, util.DefaultActionErrorWithDisplayError(decodeErr, "remote query IPC response was invalid")
}

func decodeBridgeResponse(body []byte) (map[string]interface{}, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty remote query response")
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()

	var output map[string]interface{}
	if err := decoder.Decode(&output); err != nil {
		return nil, fmt.Errorf("decode remote query response: %w", err)
	}
	status, ok := output["status"].(string)
	if !ok || status == "" {
		return nil, fmt.Errorf("remote query response missing status")
	}
	return output, nil
}
