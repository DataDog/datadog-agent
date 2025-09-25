// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_projects

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type EditProjectInputs struct {
	ProjectId     lib.GitlabID               `json:"project_id,omitempty"`
	Name          *string                    `json:"name"`
	Path          *string                    `json:"path"`
	DefaultBranch *string                    `json:"default_branch"`
	Description   *string                    `json:"description"`
	Options       *gitlab.EditProjectOptions `json:"options"`
}

type EditProjectOutputs struct {
	Project *gitlab.Project `json:"project"`
}

func (b *GitlabProjectsBundle) EditProject(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[EditProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	opts := &gitlab.EditProjectOptions{}
	if inputs.Options != nil {
		opts = inputs.Options
	}
	opts.Name = inputs.Name
	opts.Path = inputs.Path
	opts.DefaultBranch = inputs.DefaultBranch
	opts.Description = inputs.Description
	project, _, err := git.Projects.EditProject(inputs.ProjectId.String(), opts)
	if err != nil {
		return nil, err
	}
	return &EditProjectOutputs{Project: project}, nil
}
