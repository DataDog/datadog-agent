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

type CreateProjectForUserHandler struct{}

func NewCreateProjectForUserHandler() *CreateProjectForUserHandler {
	return &CreateProjectForUserHandler{}
}

type CreateProjectForUserInputs struct {
	UserId        int64                               `json:"user_id,omitempty"`
	Name          *string                             `json:"name"`
	Path          *string                             `json:"path"`
	DefaultBranch *string                             `json:"default_branch"`
	Description   *string                             `json:"description"`
	Options       *gitlab.CreateProjectForUserOptions `json:"options"`
}

type CreateProjectForUserOutputs struct {
	Project *gitlab.Project `json:"project"`
}

func (h *CreateProjectForUserHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[CreateProjectForUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	opts := &gitlab.CreateProjectForUserOptions{}
	if inputs.Options != nil {
		opts = inputs.Options
	}
	opts.Name = inputs.Name
	opts.Path = inputs.Path
	opts.DefaultBranch = inputs.DefaultBranch
	opts.Description = inputs.Description
	project, _, err := git.Projects.CreateProjectForUser(inputs.UserId, opts)
	if err != nil {
		return nil, err
	}
	return &CreateProjectForUserOutputs{Project: project}, nil
}
