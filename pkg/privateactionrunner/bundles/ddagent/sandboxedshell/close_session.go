// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sandboxedshell

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/shell/sandboxed"
)

// CloseSessionHandler handles session cleanup requests.
type CloseSessionHandler struct{}

// NewCloseSessionHandler creates a new CloseSessionHandler.
func NewCloseSessionHandler() *CloseSessionHandler {
	return &CloseSessionHandler{}
}

// CloseSessionInputs defines the input contract for the closeSession action.
type CloseSessionInputs struct {
	SessionID string `json:"sessionId"` // required
}

// CloseSessionOutputs defines the output contract for the closeSession action.
type CloseSessionOutputs struct {
	Cleaned bool `json:"cleaned"`
}

// Run cleans up an agentfs session by removing its database files.
func (h *CloseSessionHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[CloseSessionInputs](task)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("failed to extract inputs: %w", err))
	}

	if inputs.SessionID == "" {
		return nil, util.DefaultActionError(fmt.Errorf("sessionId is required"))
	}

	if err := sandboxed.CloseSession(inputs.SessionID); err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("failed to close session: %w", err))
	}

	return &CloseSessionOutputs{
		Cleaned: true,
	}, nil
}
