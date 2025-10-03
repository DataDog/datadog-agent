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

type UpdateIssueHandler struct{}

func NewUpdateIssueHandler() *UpdateIssueHandler {
	return &UpdateIssueHandler{}
}

type UpdateIssueInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	IssueIid  int          `json:"issue_iid,omitempty"`
	*gitlab.UpdateIssueOptions
}

type UpdateIssueOutputs struct {
	Issue *gitlab.Issue `json:"issue"`
}

func (h *UpdateIssueHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UpdateIssueInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issue, _, err := git.Issues.UpdateIssue(inputs.ProjectId.String(), inputs.IssueIid, inputs.UpdateIssueOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateIssueOutputs{Issue: issue}, nil
}
