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

type GetCommitStatusesInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
	*gitlab.GetCommitStatusesOptions
}

type GetCommitStatusesOutputs struct {
	CommitStatuses []*gitlab.CommitStatus `json:"commit_statuses"`
}

func (b *GitlabCommitsBundle) RunGetCommitStatuses(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetCommitStatusesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commitStatuses, _, err := git.Commits.GetCommitStatuses(inputs.ProjectId.String(), inputs.Sha, inputs.GetCommitStatusesOptions)
	if err != nil {
		return nil, err
	}
	return &GetCommitStatusesOutputs{CommitStatuses: commitStatuses}, nil
}
