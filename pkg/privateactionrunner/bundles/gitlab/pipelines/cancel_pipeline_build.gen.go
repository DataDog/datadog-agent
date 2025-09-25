// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_pipelines

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CancelPipelineBuildInputs struct {
	ProjectId  lib.GitlabID `json:"project_id,omitempty"`
	PipelineId int          `json:"pipeline_id,omitempty"`
}

type CancelPipelineBuildOutputs struct {
	Pipeline *gitlab.Pipeline `json:"pipeline"`
}

func (b *GitlabPipelinesBundle) RunCancelPipelineBuild(
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
