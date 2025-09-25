// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetProjectLanguagesInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
}

type GetProjectLanguagesOutputs struct {
	ProjectLanguages *gitlab.ProjectLanguages `json:"project_languages"`
}

func (b *GitlabProjectsBundle) GetProjectLanguages(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetProjectLanguagesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	projectLanguages, _, err := git.Projects.GetProjectLanguages(inputs.ProjectId.String())
	if err != nil {
		return nil, err
	}
	return &GetProjectLanguagesOutputs{ProjectLanguages: projectLanguages}, nil
}
