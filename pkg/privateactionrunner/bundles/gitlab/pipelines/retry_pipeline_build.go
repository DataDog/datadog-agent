// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_pipelines

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
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
	task *types.Task, credential interface{},

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
