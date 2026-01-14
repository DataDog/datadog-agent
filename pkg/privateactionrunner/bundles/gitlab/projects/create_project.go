// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_projects

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type CreateProjectHandler struct{}

func NewCreateProjectHandler() *CreateProjectHandler {
	return &CreateProjectHandler{}
}

type CreateProjectInputs struct {
	Name          *string                      `json:"name"`
	Path          *string                      `json:"path"`
	DefaultBranch *string                      `json:"default_branch"`
	Description   *string                      `json:"description"`
	Options       *gitlab.CreateProjectOptions `json:"options"`
}

type CreateProjectOutputs struct {
	Project *gitlab.Project `json:"project"`
}

func (h *CreateProjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[CreateProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	opts := &gitlab.CreateProjectOptions{}
	if inputs.Options != nil {
		opts = inputs.Options
	}
	opts.Name = inputs.Name
	opts.Path = inputs.Path
	opts.DefaultBranch = inputs.DefaultBranch
	opts.Description = inputs.Description
	project, _, err := git.Projects.CreateProject(opts)
	if err != nil {
		return nil, err
	}
	return &CreateProjectOutputs{Project: project}, nil
}
