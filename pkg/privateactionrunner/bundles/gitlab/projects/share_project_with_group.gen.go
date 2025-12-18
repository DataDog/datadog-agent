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

type ShareProjectWithGroupHandler struct{}

func NewShareProjectWithGroupHandler() *ShareProjectWithGroupHandler {
	return &ShareProjectWithGroupHandler{}
}

type ShareProjectWithGroupInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*gitlab.ShareWithGroupOptions
}

type ShareProjectWithGroupOutputs struct{}

func (h *ShareProjectWithGroupHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ShareProjectWithGroupInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Projects.ShareProjectWithGroup(inputs.ProjectId.String(), inputs.ShareWithGroupOptions)
	if err != nil {
		return nil, err
	}
	return &ShareProjectWithGroupOutputs{}, nil
}
