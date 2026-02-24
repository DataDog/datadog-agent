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

type GetPipelineTestReportHandler struct{}

func NewGetPipelineTestReportHandler() *GetPipelineTestReportHandler {
	return &GetPipelineTestReportHandler{}
}

type GetPipelineTestReportInputs struct {
	ProjectId  support.GitlabID `json:"project_id,omitempty"`
	PipelineId int64            `json:"pipeline_id,omitempty"`
}

type GetPipelineTestReportOutputs struct {
	PipelineTestReport *gitlab.PipelineTestReport `json:"pipeline_test_report"`
}

func (h *GetPipelineTestReportHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetPipelineTestReportInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	pipelineTestReport, _, err := git.Pipelines.GetPipelineTestReport(inputs.ProjectId.String(), inputs.PipelineId)
	if err != nil {
		return nil, err
	}
	return &GetPipelineTestReportOutputs{PipelineTestReport: pipelineTestReport}, nil
}
