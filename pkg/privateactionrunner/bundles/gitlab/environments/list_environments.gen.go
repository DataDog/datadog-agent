// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_environments

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type ListEnvironmentsHandler struct{}

func NewListEnvironmentsHandler() *ListEnvironmentsHandler {
	return &ListEnvironmentsHandler{}
}

type ListEnvironmentsInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListEnvironmentsOptions
}

type ListEnvironmentsOutputs struct {
	Environments []*gitlab.Environment `json:"environments"`
}

func (h *ListEnvironmentsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListEnvironmentsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	environments, _, err := git.Environments.ListEnvironments(inputs.ProjectId.String(), inputs.ListEnvironmentsOptions)
	if err != nil {
		return nil, err
	}
	return &ListEnvironmentsOutputs{Environments: environments}, nil
}
