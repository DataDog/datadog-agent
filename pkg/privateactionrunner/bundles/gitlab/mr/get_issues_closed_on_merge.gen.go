// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetIssuesClosedOnMergeHandler struct{}

func NewGetIssuesClosedOnMergeHandler() *GetIssuesClosedOnMergeHandler {
	return &GetIssuesClosedOnMergeHandler{}
}

type GetIssuesClosedOnMergeInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	*gitlab.GetIssuesClosedOnMergeOptions
}

type GetIssuesClosedOnMergeOutputs struct {
	Issues []*gitlab.Issue `json:"issues"`
}

func (h *GetIssuesClosedOnMergeHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetIssuesClosedOnMergeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issues, _, err := git.MergeRequests.GetIssuesClosedOnMerge(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.GetIssuesClosedOnMergeOptions)
	if err != nil {
		return nil, err
	}
	return &GetIssuesClosedOnMergeOutputs{Issues: issues}, nil
}
