// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_pipelines

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ListProjectPipelinesHandler struct{}

func NewListProjectPipelinesHandler() *ListProjectPipelinesHandler {
	return &ListProjectPipelinesHandler{}
}

type ListProjectPipelinesInputs struct {
	ProjectID support.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListProjectPipelinesOptions
}

type ListProjectPipelinesOutputs struct {
	Pipelines []*gitlab.PipelineInfo `json:"pipelines"`
}

func (h *ListProjectPipelinesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListProjectPipelinesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}
	pipelines, _, err := git.Pipelines.ListProjectPipelines(inputs.ProjectID.String(), inputs.ListProjectPipelinesOptions)
	if err != nil {
		return nil, fmt.Errorf("could not list project pipelines: %w", err)
	}
	return &ListProjectPipelinesOutputs{Pipelines: pipelines}, nil
}
