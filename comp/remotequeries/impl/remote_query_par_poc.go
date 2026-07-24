// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
)

const (
	// AgentRemoteQueryExecuteEndpointPath is the POC-only caller path for the core-Agent command API mount.
	// This deliberately documents the current dev proof shape and is not a production IPC API commitment.
	AgentRemoteQueryExecuteEndpointPath = "/agent" + RemoteQueryExecuteEndpointPath
)

// remoteQueryPARIPCClient is the narrow Agent IPC client surface this POC caller needs.
type remoteQueryPARIPCClient interface {
	Post(url string, contentType string, body io.Reader, opts ...ipc.RequestOption) (resp []byte, err error)
}

// RemoteQueryPARHarness is a dev-only, PAR-shaped proof caller for the remote query execute bridge.
// It accepts credential-free action-like inputs, sends them through the injected Agent IPC HTTP client,
// and decodes the execute bridge response without depending on PAR's production runner/bundle registry.
type RemoteQueryPARHarness struct {
	client      remoteQueryPARIPCClient
	endpointURL string
}

// RemoteQueryPARInputs is the credential-free task input shape for the PAR-shaped POC harness.
type RemoteQueryPARInputs struct {
	Integration string                            `json:"integration"`
	Operation   string                            `json:"operation"`
	Format      string                            `json:"format"`
	Target      remoteQueryTargetJSON             `json:"target"`
	Query       string                            `json:"query"`
	CopyLimits  *remoteQueryExecuteCopyLimitsJSON `json:"copyLimits,omitempty"`
}

// RemoteQueryPARResult is the decoded execute bridge result or sanitized bridge error.
type RemoteQueryPARResult struct {
	HTTPStatus int              `json:"-"`
	Status     string           `json:"status"`
	Rows       []map[string]any `json:"rows,omitempty"`
	Error      *responseError   `json:"error,omitempty"`
	Raw        json.RawMessage  `json:"-"`
}

// NewRemoteQueryPARHarness creates a dev-only PAR-shaped IPC execute proof caller.
func NewRemoteQueryPARHarness(client remoteQueryPARIPCClient, endpointURL string) *RemoteQueryPARHarness {
	return &RemoteQueryPARHarness{client: client, endpointURL: endpointURL}
}

// Execute sends a credential-free target/query request through the injected Agent IPC HTTP client.
func (h *RemoteQueryPARHarness) Execute(ctx context.Context, inputs RemoteQueryPARInputs) (RemoteQueryPARResult, error) {
	if h == nil || h.client == nil {
		return RemoteQueryPARResult{}, errors.New("remote query PAR harness requires an IPC client")
	}
	if h.endpointURL == "" {
		return RemoteQueryPARResult{}, errors.New("remote query PAR harness requires an endpoint URL")
	}

	payload, err := json.Marshal(inputs)
	if err != nil {
		return RemoteQueryPARResult{}, fmt.Errorf("marshal remote query PAR inputs: %w", err)
	}

	body, postErr := h.client.Post(h.endpointURL, "application/json", bytes.NewReader(payload), ipchttp.WithContext(ctx))
	result, decodeErr := decodeRemoteQueryPARResponse(body)
	if decodeErr == nil {
		// IPC HTTPClient returns both the response body and an error for HTTP >= 400.
		// The execute bridge body is the sanitized contract, so propagate that decoded body.
		return result, nil
	}
	if postErr != nil {
		if len(body) > 0 {
			return RemoteQueryPARResult{}, errors.New("remote query IPC request failed with undecodable response")
		}
		return RemoteQueryPARResult{}, fmt.Errorf("remote query IPC request failed: %w", postErr)
	}
	return RemoteQueryPARResult{}, decodeErr
}

func decodeRemoteQueryPARResponse(body []byte) (RemoteQueryPARResult, error) {
	if len(body) == 0 {
		return RemoteQueryPARResult{}, errors.New("empty remote query response")
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()

	var result RemoteQueryPARResult
	if err := decoder.Decode(&result); err != nil {
		return RemoteQueryPARResult{}, fmt.Errorf("decode remote query response: %w", err)
	}
	if result.Status == "" {
		return RemoteQueryPARResult{}, errors.New("remote query response missing status")
	}
	result.Raw = append(result.Raw[:0], body...)
	return result, nil
}
