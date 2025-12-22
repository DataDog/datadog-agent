// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_temporal

import (
	"context"

	v112 "go.temporal.io/api/workflow/v1"
	workflowService "go.temporal.io/api/workflowservice/v1"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ListWorkflowsHandler struct{}

func NewListWorkflowsHandler() *ListWorkflowsHandler {
	return &ListWorkflowsHandler{}
}

type ListWorkflowsInputs struct {
	Query     string `json:"query,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type ListWorkflowsOutputs struct {
	Workflows []*v112.WorkflowExecutionInfo `json:"workflows"`
}

func (h *ListWorkflowsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[ListWorkflowsInputs](task)
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

	request := &workflowService.ListWorkflowExecutionsRequest{
		Namespace: namespace,
		Query:     inputs.Query,
	}

	workflows, err := temporalClient.ListWorkflow(ctx, request)
	if err != nil {
		log.FromContext(ctx).Warn("Unable to list workflows.")
		return nil, err
	}

	workflowsExecutions := []*v112.WorkflowExecutionInfo{}

	if workflows != nil {
		workflowsExecutions = workflows.Executions
	}

	return &ListWorkflowsOutputs{
		Workflows: workflowsExecutions,
	}, nil
}
