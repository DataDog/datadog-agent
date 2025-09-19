package com_datadoghq_gitlab_pipelines

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
)

type ListProjectPipelinesHandler struct{}

func NewListProjectPipelinesHandler() *ListProjectPipelinesHandler {
	return &ListProjectPipelinesHandler{}
}

type ListProjectPipelinesInputs struct {
	ProjectID lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListProjectPipelinesOptions
}

type ListProjectPipelinesOutputs struct {
	Pipelines []*gitlab.PipelineInfo `json:"pipelines"`
}

func (h *ListProjectPipelinesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *runtimepb.Credential,
) (any, error) {
	inputs, err := types.ExtractInputs[ListProjectPipelinesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}
	pipelines, _, err := git.Pipelines.ListProjectPipelines(inputs.ProjectID.String(), inputs.ListProjectPipelinesOptions)
	if err != nil {
		return nil, fmt.Errorf("could not list project pipelines: %w", err)
	}
	return &ListProjectPipelinesOutputs{Pipelines: pipelines}, nil
}
