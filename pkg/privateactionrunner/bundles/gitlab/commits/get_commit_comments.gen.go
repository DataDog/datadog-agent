// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_commits

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GetCommitCommentsHandler struct{}

func NewGetCommitCommentsHandler() *GetCommitCommentsHandler {
	return &GetCommitCommentsHandler{}
}

type GetCommitCommentsInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	Sha       string           `json:"sha,omitempty"`
	*gitlab.GetCommitCommentsOptions
}

type GetCommitCommentsOutputs struct {
	CommitComments []*gitlab.CommitComment `json:"commit_comments"`
}

func (h *GetCommitCommentsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetCommitCommentsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commitComments, _, err := git.Commits.GetCommitComments(inputs.ProjectId.String(), inputs.Sha, inputs.GetCommitCommentsOptions)
	if err != nil {
		return nil, err
	}
	return &GetCommitCommentsOutputs{CommitComments: commitComments}, nil
}
