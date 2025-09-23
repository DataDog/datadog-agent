// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_commits

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RevertCommitHandler struct{}

func NewRevertCommitHandler() *RevertCommitHandler {
	return &RevertCommitHandler{}
}

type RevertCommitInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
	*gitlab.RevertCommitOptions
}

type RevertCommitOutputs struct {
	Commit *gitlab.Commit `json:"commit"`
}

func (h *RevertCommitHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[RevertCommitInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}
	commit, _, err := git.Commits.RevertCommit(inputs.ProjectId.String(), inputs.Sha, inputs.RevertCommitOptions)
	if err != nil {
		return nil, err
	}
	return &RevertCommitOutputs{Commit: commit}, nil
}
