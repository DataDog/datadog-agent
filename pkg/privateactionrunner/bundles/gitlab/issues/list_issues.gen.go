// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_issues

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type ListIssuesHandler struct{}

func NewListIssuesHandler() *ListIssuesHandler {
	return &ListIssuesHandler{}
}

type ListIssuesInputs struct {
	*gitlab.ListIssuesOptions
}

type ListIssuesOutputs struct {
	Issues []*gitlab.Issue `json:"issues"`
}

func (h *ListIssuesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListIssuesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issues, _, err := git.Issues.ListIssues(inputs.ListIssuesOptions)
	if err != nil {
		return nil, err
	}
	return &ListIssuesOutputs{Issues: issues}, nil
}
