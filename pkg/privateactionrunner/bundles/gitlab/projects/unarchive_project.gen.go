// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_projects

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type UnarchiveProjectHandler struct{}

func NewUnarchiveProjectHandler() *UnarchiveProjectHandler {
	return &UnarchiveProjectHandler{}
}

type UnarchiveProjectInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
}

type UnarchiveProjectOutputs struct {
	Project *gitlab.Project `json:"project"`
}

func (h *UnarchiveProjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[UnarchiveProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	project, _, err := git.Projects.UnarchiveProject(inputs.ProjectId.String())
	if err != nil {
		return nil, err
	}
	return &UnarchiveProjectOutputs{Project: project}, nil
}
