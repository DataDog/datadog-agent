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
)

type DeletePipelineHandler struct{}

func NewDeletePipelineHandler() *DeletePipelineHandler {
	return &DeletePipelineHandler{}
}

type DeletePipelineInputs struct {
	ProjectId  support.GitlabID `json:"project_id,omitempty"`
	PipelineId int64            `json:"pipeline_id,omitempty"`
}

type DeletePipelineOutputs struct{}

func (h *DeletePipelineHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[DeletePipelineInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Pipelines.DeletePipeline(inputs.ProjectId.String(), inputs.PipelineId)
	if err != nil {
		return nil, err
	}
	return &DeletePipelineOutputs{}, nil
}
