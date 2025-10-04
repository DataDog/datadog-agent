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

type ListMergeRequestsRelatedToIssueHandler struct{}

func NewListMergeRequestsRelatedToIssueHandler() *ListMergeRequestsRelatedToIssueHandler {
	return &ListMergeRequestsRelatedToIssueHandler{}
}

type ListMergeRequestsRelatedToIssueInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	IssueIid  int          `json:"issue_iid,omitempty"`
	*gitlab.ListMergeRequestsRelatedToIssueOptions
}

type ListMergeRequestsRelatedToIssueOutputs struct {
	BasicMergeRequests []*gitlab.BasicMergeRequest `json:"merge_requests"`
}

func (h *ListMergeRequestsRelatedToIssueHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListMergeRequestsRelatedToIssueInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	basicMergeRequests, _, err := git.Issues.ListMergeRequestsRelatedToIssue(inputs.ProjectId.String(), inputs.IssueIid, inputs.ListMergeRequestsRelatedToIssueOptions)
	if err != nil {
		return nil, err
	}
	return &ListMergeRequestsRelatedToIssueOutputs{BasicMergeRequests: basicMergeRequests}, nil
}
