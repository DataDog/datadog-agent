// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_commits

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListCommitsHandler struct{}

func NewListCommitsHandler() *ListCommitsHandler {
	return &ListCommitsHandler{}
}

type ListCommitsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListCommitsOptions
}

type ListCommitsOutputs struct {
	Commits []*gitlab.Commit `json:"commits"`
}

func (h *ListCommitsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListCommitsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commits, _, err := git.Commits.ListCommits(inputs.ProjectId.String(), inputs.ListCommitsOptions)
	if err != nil {
		return nil, err
	}
	return &ListCommitsOutputs{Commits: commits}, nil
}
