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

type GetLatestPipelineHandler struct{}

func NewGetLatestPipelineHandler() *GetLatestPipelineHandler {
	return &GetLatestPipelineHandler{}
}

type GetLatestPipelineInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*gitlab.GetLatestPipelineOptions
}

type GetLatestPipelineOutputs struct {
	Pipeline *gitlab.Pipeline `json:"pipeline"`
}

func (h *GetLatestPipelineHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetLatestPipelineInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	pipeline, _, err := git.Pipelines.GetLatestPipeline(inputs.ProjectId.String(), inputs.GetLatestPipelineOptions)
	if err != nil {
		return nil, err
	}
	return &GetLatestPipelineOutputs{Pipeline: pipeline}, nil
}
