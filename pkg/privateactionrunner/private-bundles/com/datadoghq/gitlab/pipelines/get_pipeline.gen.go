package com_datadoghq_gitlab_pipelines

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetPipelineHandler struct{}

func NewGetPipelineHandler() *GetPipelineHandler {
	return &GetPipelineHandler{}
}

type GetPipelineInputs struct {
	ProjectId  lib.GitlabID `json:"project_id,omitempty"`
	PipelineId int          `json:"pipeline_id,omitempty"`
}

type GetPipelineOutputs struct {
	Pipeline *gitlab.Pipeline `json:"pipeline"`
}

func (h *GetPipelineHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetPipelineInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	pipeline, _, err := git.Pipelines.GetPipeline(inputs.ProjectId.String(), inputs.PipelineId)
	if err != nil {
		return nil, err
	}
	return &GetPipelineOutputs{Pipeline: pipeline}, nil
}
