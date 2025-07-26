// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_temporal

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type GetWorkflowResultHandler struct{}

func NewGetWorkflowResultHandler() *GetWorkflowResultHandler {
	return &GetWorkflowResultHandler{}
}

type GetWorkflowResultInputs struct {
	RunId      string `json:"runId,omitempty"`
	WorkflowId string `json:"workflowId,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
}

type GetWorkflowResultOutputs struct {
	ExecutionResult string `json:"executionResult"`
}

func (h *GetWorkflowResultHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials interface{},
) (interface{}, error) {
	inputs, err := types.ExtractInputs[GetWorkflowResultInputs](task)
	if err != nil {
		return nil, err
	}

	namespace := inputs.Namespace
	if inputs.Namespace == "" {
		namespace = "default"
	}
	temporalClient, err := newTemporalClient(ctx, credentials, namespace)
	if err != nil {
		return nil, err
	}
	defer temporalClient.Close()

	workflowRun := temporalClient.GetWorkflow(ctx, inputs.WorkflowId, inputs.RunId)
	if workflowRun == nil {
		log.Warn("Unable to get workflow run.")
		return nil, fmt.Errorf("unable to get workflow run")
	}
	var workflowResult string
	err = workflowRun.Get(ctx, &workflowResult)
	if err != nil {
		return nil, err
	}
	return &GetWorkflowResultOutputs{
		ExecutionResult: workflowResult,
	}, nil
}
