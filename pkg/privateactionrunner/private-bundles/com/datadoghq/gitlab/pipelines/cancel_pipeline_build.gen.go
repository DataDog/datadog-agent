package com_datadoghq_gitlab_pipelines

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CancelPipelineBuildHandler struct{}

func NewCancelPipelineBuildHandler() *CancelPipelineBuildHandler {
	return &CancelPipelineBuildHandler{}
}

type CancelPipelineBuildInputs struct {
	ProjectId  lib.GitlabID `json:"project_id,omitempty"`
	PipelineId int          `json:"pipeline_id,omitempty"`
}

type CancelPipelineBuildOutputs struct {
	Pipeline *gitlab.Pipeline `json:"pipeline"`
}

func (h *CancelPipelineBuildHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CancelPipelineBuildInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	pipeline, _, err := git.Pipelines.CancelPipelineBuild(inputs.ProjectId.String(), inputs.PipelineId)
	if err != nil {
		return nil, err
	}
	return &CancelPipelineBuildOutputs{Pipeline: pipeline}, nil
}
