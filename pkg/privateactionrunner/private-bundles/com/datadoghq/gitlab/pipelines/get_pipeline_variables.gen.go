package com_datadoghq_gitlab_pipelines

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetPipelineVariablesHandler struct{}

func NewGetPipelineVariablesHandler() *GetPipelineVariablesHandler {
	return &GetPipelineVariablesHandler{}
}

type GetPipelineVariablesInputs struct {
	ProjectId  lib.GitlabID `json:"project_id,omitempty"`
	PipelineId int          `json:"pipeline_id,omitempty"`
	Page       int          `json:"page,omitempty"`
	PerPage    int          `json:"per_page,omitempty"`
}

type GetPipelineVariablesOutputs struct {
	PipelineVariables []*gitlab.PipelineVariable `json:"pipeline_variables"`
}

func (h *GetPipelineVariablesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetPipelineVariablesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	pipelineVariables, _, err := git.Pipelines.GetPipelineVariables(inputs.ProjectId.String(), inputs.PipelineId, lib.WithPagination(inputs.Page, inputs.PerPage))
	if err != nil {
		return nil, err
	}
	return &GetPipelineVariablesOutputs{PipelineVariables: pipelineVariables}, nil
}
