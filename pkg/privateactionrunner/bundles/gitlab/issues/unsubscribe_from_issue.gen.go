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

type UnsubscribeFromIssueHandler struct{}

func NewUnsubscribeFromIssueHandler() *UnsubscribeFromIssueHandler {
	return &UnsubscribeFromIssueHandler{}
}

type UnsubscribeFromIssueInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	IssueIid  int64            `json:"issue_iid,omitempty"`
}

type UnsubscribeFromIssueOutputs struct {
	Issue *gitlab.Issue `json:"issue"`
}

func (h *UnsubscribeFromIssueHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[UnsubscribeFromIssueInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issue, _, err := git.Issues.UnsubscribeFromIssue(inputs.ProjectId.String(), inputs.IssueIid)
	if err != nil {
		return nil, err
	}
	return &UnsubscribeFromIssueOutputs{Issue: issue}, nil
}
