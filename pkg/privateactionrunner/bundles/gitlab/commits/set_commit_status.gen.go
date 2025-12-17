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

type SetCommitStatusHandler struct{}

func NewSetCommitStatusHandler() *SetCommitStatusHandler {
	return &SetCommitStatusHandler{}
}

type SetCommitStatusInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	Sha       string           `json:"sha,omitempty"`
	*gitlab.SetCommitStatusOptions
}

type SetCommitStatusOutputs struct {
	CommitStatus *gitlab.CommitStatus `json:"commit_status"`
}

func (h *SetCommitStatusHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[SetCommitStatusInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commitStatus, _, err := git.Commits.SetCommitStatus(inputs.ProjectId.String(), inputs.Sha, inputs.SetCommitStatusOptions)
	if err != nil {
		return nil, err
	}
	return &SetCommitStatusOutputs{CommitStatus: commitStatus}, nil
}
