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

type ListMergeRequestsByCommitHandler struct{}

func NewListMergeRequestsByCommitHandler() *ListMergeRequestsByCommitHandler {
	return &ListMergeRequestsByCommitHandler{}
}

type ListMergeRequestsByCommitInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	Sha       string           `json:"sha,omitempty"`
}

type ListMergeRequestsByCommitOutputs struct {
	BasicMergeRequests []*gitlab.BasicMergeRequest `json:"merge_requests"`
}

func (h *ListMergeRequestsByCommitHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListMergeRequestsByCommitInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	basicMergeRequests, _, err := git.Commits.ListMergeRequestsByCommit(inputs.ProjectId.String(), inputs.Sha)
	if err != nil {
		return nil, err
	}
	return &ListMergeRequestsByCommitOutputs{BasicMergeRequests: basicMergeRequests}, nil
}
