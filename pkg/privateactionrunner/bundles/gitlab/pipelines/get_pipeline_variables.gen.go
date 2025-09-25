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

type GetPipelineVariablesInputs struct {
	ProjectId  lib.GitlabID `json:"project_id,omitempty"`
	PipelineId int          `json:"pipeline_id,omitempty"`
	Page       int          `json:"page,omitempty"`
	PerPage    int          `json:"per_page,omitempty"`
}

type GetPipelineVariablesOutputs struct {
	PipelineVariables []*gitlab.PipelineVariable `json:"pipeline_variables"`
}

func (b *GitlabPipelinesBundle) RunGetPipelineVariables(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetPipelineVariablesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	pipelineVariables, _, err := git.Pipelines.GetPipelineVariables(inputs.ProjectId.String(), inputs.PipelineId, lib.WithPagination(inputs.Page, inputs.PerPage))
	if err != nil {
		return nil, err
	}
	return &GetPipelineVariablesOutputs{PipelineVariables: pipelineVariables}, nil
}
