package com_datadoghq_gitlab_pipelines

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
)

type RetryPipelineBuildHandler struct{}

func NewRetryPipelineBuildHandler() *RetryPipelineBuildHandler {
	return &RetryPipelineBuildHandler{}
}

type RetryPipelineBuildInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Pipeline  int          `json:"pipeline_id,omitempty"`
}

type RetryPipelineBuildOutputs struct {
	Pipeline *gitlab.Pipeline `json:"pipeline"`
}

func (h *RetryPipelineBuildHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *runtimepb.Credential,
) (any, error) {
	inputs, err := types.ExtractInputs[RetryPipelineBuildInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}
	pipeline, _, err := git.Pipelines.RetryPipelineBuild(inputs.ProjectId.String(), inputs.Pipeline)
	if err != nil {
		return nil, err
	}
	return &RetryPipelineBuildOutputs{Pipeline: pipeline}, nil
}
