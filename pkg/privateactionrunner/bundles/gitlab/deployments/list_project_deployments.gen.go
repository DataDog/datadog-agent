// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_deployments

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListProjectDeploymentsHandler struct{}

func NewListProjectDeploymentsHandler() *ListProjectDeploymentsHandler {
	return &ListProjectDeploymentsHandler{}
}

type ListProjectDeploymentsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListProjectDeploymentsOptions
}

type ListProjectDeploymentsOutputs struct {
	Deployments []*gitlab.Deployment `json:"deployments"`
}

func (h *ListProjectDeploymentsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListProjectDeploymentsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	deployments, _, err := git.Deployments.ListProjectDeployments(inputs.ProjectId.String(), inputs.ListProjectDeploymentsOptions)
	if err != nil {
		return nil, err
	}
	return &ListProjectDeploymentsOutputs{Deployments: deployments}, nil
}
