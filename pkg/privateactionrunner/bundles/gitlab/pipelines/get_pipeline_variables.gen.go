// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_pipelines

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GetPipelineVariablesHandler struct{}

func NewGetPipelineVariablesHandler() *GetPipelineVariablesHandler {
	return &GetPipelineVariablesHandler{}
}

type GetPipelineVariablesInputs struct {
	ProjectId  support.GitlabID `json:"project_id,omitempty"`
	PipelineId int64            `json:"pipeline_id,omitempty"`
	Page       int              `json:"page,omitempty"`
	PerPage    int              `json:"per_page,omitempty"`
}

type GetPipelineVariablesOutputs struct {
	PipelineVariables []*gitlab.PipelineVariable `json:"pipeline_variables"`
}

func (h *GetPipelineVariablesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetPipelineVariablesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	pipelineVariables, _, err := git.Pipelines.GetPipelineVariables(inputs.ProjectId.String(), inputs.PipelineId, support.WithPagination(inputs.Page, inputs.PerPage))
	if err != nil {
		return nil, err
	}
	return &GetPipelineVariablesOutputs{PipelineVariables: pipelineVariables}, nil
}
