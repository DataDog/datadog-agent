// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkdevices

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestTriggerDiscoveryHandlerRejectsMalformedInputs(t *testing.T) {
	h := NewTriggerDiscoveryHandler(ipcmock.New(t).GetClient())

	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{Inputs: map[string]any{"autodiscoveryId": []string{"not-a-string"}}}

	_, err := h.Run(context.Background(), task, nil)

	require.ErrorContains(t, err, "failed to parse triggerNdmDiscovery inputs")
}

func TestTriggerDiscoveryHandlerNeedsAnIPCClient(t *testing.T) {
	h := NewTriggerDiscoveryHandler(nil)

	_, err := h.Run(context.Background(), &types.Task{}, nil)

	require.ErrorContains(t, err, "IPC client is not available")
}

func TestRefusalReasonReadsTheAgentsError(t *testing.T) {
	assert.Equal(t, "a sweep is already running: range \"ad-1\" is already sweeping",
		refusalReason([]byte(`{"error":"a sweep is already running: range \"ad-1\" is already sweeping"}`)))
}

func TestRefusalReasonIsEmptyForAnythingButAnErrorBody(t *testing.T) {
	assert.Empty(t, refusalReason(nil), "an unreachable Agent is a task failure, not a refusal")
	assert.Empty(t, refusalReason([]byte("502 Bad Gateway")), "a non-JSON body is not a refusal")
	assert.Empty(t, refusalReason([]byte(`{"other":"field"}`)), "a body with no error is not a refusal")
}
