// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repositories

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type MergeBaseHandler struct{}

func NewMergeBaseHandler() *MergeBaseHandler {
	return &MergeBaseHandler{}
}

type MergeBaseInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*gitlab.MergeBaseOptions
}

type MergeBaseOutputs struct {
	Commit *gitlab.Commit `json:"commit"`
}

func (h *MergeBaseHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[MergeBaseInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commit, _, err := git.Repositories.MergeBase(inputs.ProjectId.String(), inputs.MergeBaseOptions)
	if err != nil {
		return nil, err
	}
	return &MergeBaseOutputs{Commit: commit}, nil
}
