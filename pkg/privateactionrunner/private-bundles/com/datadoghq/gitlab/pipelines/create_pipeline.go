package com_datadoghq_gitlab_pipelines

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
)

type CreatePipelineHandler struct{}

func NewCreatePipelineHandler() *CreatePipelineHandler {
	return &CreatePipelineHandler{}
}

type CreatePipelineInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.CreatePipelineOptions
}

type CreatePipelineOutputs struct {
	Pipeline *gitlab.Pipeline `json:"pipeline"`
}

func (h *CreatePipelineHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *runtimepb.Credential,
) (any, error) {
	inputs, err := types.ExtractInputs[CreatePipelineInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	pipeline, _, err := git.Pipelines.CreatePipeline(inputs.ProjectId.String(), inputs.CreatePipelineOptions)
	if err != nil {
		return nil, err
	}
	return &CreatePipelineOutputs{Pipeline: pipeline}, nil
}
