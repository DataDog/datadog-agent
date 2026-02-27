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
)

type DeleteIssueHandler struct{}

func NewDeleteIssueHandler() *DeleteIssueHandler {
	return &DeleteIssueHandler{}
}

type DeleteIssueInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	IssueIid  int64            `json:"issue_iid,omitempty"`
}

type DeleteIssueOutputs struct{}

func (h *DeleteIssueHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[DeleteIssueInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Issues.DeleteIssue(inputs.ProjectId.String(), inputs.IssueIid)
	if err != nil {
		return nil, err
	}
	return &DeleteIssueOutputs{}, nil
}
