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

type SetCommitStatusInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
	*gitlab.SetCommitStatusOptions
}

type SetCommitStatusOutputs struct {
	CommitStatus *gitlab.CommitStatus `json:"commit_status"`
}

func (b *GitlabCommitsBundle) RunSetCommitStatus(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[SetCommitStatusInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commitStatus, _, err := git.Commits.SetCommitStatus(inputs.ProjectId.String(), inputs.Sha, inputs.SetCommitStatusOptions)
	if err != nil {
		return nil, err
	}
	return &SetCommitStatusOutputs{CommitStatus: commitStatus}, nil
}
