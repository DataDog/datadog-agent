// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_temporal

import (
	"context"
	"errors"

	"go.temporal.io/sdk/client"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RunWorkflowHandler struct{}

func NewRunWorkflowHandler() *RunWorkflowHandler {
	return &RunWorkflowHandler{}
}

type RunWorkflowInputs struct {
	WorkFlowType string `json:"workflowType,omitempty"`
	WorkflowArgs []any  `json:"workflowArgs,omitempty"`
	WorkflowId   string `json:"workflowId,omitempty"`
	TaskQueue    string `json:"taskQueue,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
}

type RunWorkflowOutputs struct {
	RunId string `json:"runId"`
}

func (h *RunWorkflowHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RunWorkflowInputs](task)
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

	options := client.StartWorkflowOptions{
		ID:        inputs.WorkflowId,
		TaskQueue: inputs.TaskQueue,
	}

	workflowExecution, err := temporalClient.ExecuteWorkflow(ctx, options, inputs.WorkFlowType, inputs.WorkflowArgs...)
	if err != nil {
		log.FromContext(ctx).Warn("Unable to run workflow.")
		return nil, err
	}
	if workflowExecution == nil {
		return nil, errors.New("workflow execution not found")
	}

	runId := workflowExecution.GetRunID()
	return &RunWorkflowOutputs{
		RunId: runId,
	}, nil
}
