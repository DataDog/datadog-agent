// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_issues

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type SetTimeEstimateHandler struct{}

func NewSetTimeEstimateHandler() *SetTimeEstimateHandler {
	return &SetTimeEstimateHandler{}
}

type SetTimeEstimateInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	IssueIid  int          `json:"issue_iid,omitempty"`
	*gitlab.SetTimeEstimateOptions
}

type SetTimeEstimateOutputs struct {
	TimeStats *gitlab.TimeStats `json:"time_stats"`
}

func (h *SetTimeEstimateHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[SetTimeEstimateInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	timeStats, _, err := git.Issues.SetTimeEstimate(inputs.ProjectId.String(), inputs.IssueIid, inputs.SetTimeEstimateOptions)
	if err != nil {
		return nil, err
	}
	return &SetTimeEstimateOutputs{TimeStats: timeStats}, nil
}
