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

type PostCommitCommentHandler struct{}

func NewPostCommitCommentHandler() *PostCommitCommentHandler {
	return &PostCommitCommentHandler{}
}

type PostCommitCommentInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
	*gitlab.PostCommitCommentOptions
}

type PostCommitCommentOutputs struct {
	CommitComment *gitlab.CommitComment `json:"commit_comment"`
}

func (h *PostCommitCommentHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[PostCommitCommentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commitComment, _, err := git.Commits.PostCommitComment(inputs.ProjectId.String(), inputs.Sha, inputs.PostCommitCommentOptions)
	if err != nil {
		return nil, err
	}
	return &PostCommitCommentOutputs{CommitComment: commitComment}, nil
}
