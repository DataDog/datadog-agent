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

type ListUserContributedProjectsHandler struct{}

func NewListUserContributedProjectsHandler() *ListUserContributedProjectsHandler {
	return &ListUserContributedProjectsHandler{}
}

type ListUserContributedProjectsInputs struct {
	UserId lib.GitlabID `json:"user_id,omitempty"`
	*gitlab.ListProjectsOptions
}

type ListUserContributedProjectsOutputs struct {
	Projects []*gitlab.Project `json:"projects"`
}

func (h *ListUserContributedProjectsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListUserContributedProjectsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	projects, _, err := git.Projects.ListUserContributedProjects(inputs.UserId.String(), inputs.ListProjectsOptions)
	if err != nil {
		return nil, err
	}
	return &ListUserContributedProjectsOutputs{Projects: projects}, nil
}
