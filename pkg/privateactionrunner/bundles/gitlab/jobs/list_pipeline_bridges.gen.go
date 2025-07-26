// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_jobs

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListPipelineBridgesHandler struct{}

func NewListPipelineBridgesHandler() *ListPipelineBridgesHandler {
	return &ListPipelineBridgesHandler{}
}

type ListPipelineBridgesInputs struct {
	ProjectId  lib.GitlabID `json:"project_id,omitempty"`
	PipelineId int          `json:"pipeline_id,omitempty"`
	*gitlab.ListJobsOptions
}

type ListPipelineBridgesOutputs struct {
	Bridges []*gitlab.Bridge `json:"bridges"`
}

func (h *ListPipelineBridgesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListPipelineBridgesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	bridges, _, err := git.Jobs.ListPipelineBridges(inputs.ProjectId.String(), inputs.PipelineId, inputs.ListJobsOptions)
	if err != nil {
		return nil, err
	}
	return &ListPipelineBridgesOutputs{Bridges: bridges}, nil
}
