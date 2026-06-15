// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkdevices

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// ConnectivityCheckHandler implements the connectivityCheck PAR action.
type ConnectivityCheckHandler struct{}

// NewConnectivityCheckHandler creates a new ConnectivityCheckHandler.
func NewConnectivityCheckHandler() *ConnectivityCheckHandler {
	return &ConnectivityCheckHandler{}
}

// Run parses the action inputs and runs the connectivity check.
func (h *ConnectivityCheckHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	req, err := types.ExtractInputs[Request](task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connectivityCheck inputs: %w", err)
	}

	out, err := runChecks(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("connectivityCheck: %w", err)
	}
	return out, nil
}
