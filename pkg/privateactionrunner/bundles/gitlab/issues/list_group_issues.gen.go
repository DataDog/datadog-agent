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

type ListGroupIssuesHandler struct{}

func NewListGroupIssuesHandler() *ListGroupIssuesHandler {
	return &ListGroupIssuesHandler{}
}

type ListGroupIssuesInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListGroupIssuesOptions
}

type ListGroupIssuesOutputs struct {
	Issues []*gitlab.Issue `json:"issues"`
}

func (h *ListGroupIssuesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListGroupIssuesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issues, _, err := git.Issues.ListGroupIssues(inputs.ProjectId.String(), inputs.ListGroupIssuesOptions)
	if err != nil {
		return nil, err
	}
	return &ListGroupIssuesOutputs{Issues: issues}, nil
}
