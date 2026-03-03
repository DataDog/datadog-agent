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

type UpdatePipelineMetadataHandler struct{}

func NewUpdatePipelineMetadataHandler() *UpdatePipelineMetadataHandler {
	return &UpdatePipelineMetadataHandler{}
}

type UpdatePipelineMetadataInputs struct {
	ProjectId  support.GitlabID `json:"project_id,omitempty"`
	PipelineId int64            `json:"pipeline_id,omitempty"`
	*gitlab.UpdatePipelineMetadataOptions
}

type UpdatePipelineMetadataOutputs struct {
	Pipeline *gitlab.Pipeline `json:"pipeline"`
}

func (h *UpdatePipelineMetadataHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[UpdatePipelineMetadataInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	pipeline, _, err := git.Pipelines.UpdatePipelineMetadata(inputs.ProjectId.String(), inputs.PipelineId, inputs.UpdatePipelineMetadataOptions)
	if err != nil {
		return nil, err
	}
	return &UpdatePipelineMetadataOutputs{Pipeline: pipeline}, nil
}
