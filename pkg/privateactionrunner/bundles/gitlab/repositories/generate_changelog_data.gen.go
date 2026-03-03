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

type GenerateChangelogDataHandler struct{}

func NewGenerateChangelogDataHandler() *GenerateChangelogDataHandler {
	return &GenerateChangelogDataHandler{}
}

type GenerateChangelogDataInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	gitlab.GenerateChangelogDataOptions
}

type GenerateChangelogDataOutputs struct {
	ChangelogData *gitlab.ChangelogData `json:"changelog_data"`
}

func (h *GenerateChangelogDataHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GenerateChangelogDataInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	changelogData, _, err := git.Repositories.GenerateChangelogData(inputs.ProjectId.String(), inputs.GenerateChangelogDataOptions)
	if err != nil {
		return nil, err
	}
	return &GenerateChangelogDataOutputs{ChangelogData: changelogData}, nil
}
