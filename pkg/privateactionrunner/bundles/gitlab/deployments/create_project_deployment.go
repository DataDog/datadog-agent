// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_deployments

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type CreateProjectDeploymentHandler struct{}

func NewCreateProjectDeploymentHandler() *CreateProjectDeploymentHandler {
	return &CreateProjectDeploymentHandler{}
}

type CreateProjectDeploymentInputs struct {
	ProjectID support.GitlabID `json:"project_id,omitempty"`
	*gitlab.CreateProjectDeploymentOptions
}

type CreateProjectDeploymentOutputs struct {
	Deployment *gitlab.Deployment `json:"deployment"`
}

func (h *CreateProjectDeploymentHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[CreateProjectDeploymentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}
	deployment, _, err := git.Deployments.CreateProjectDeployment(inputs.ProjectID.String(), inputs.CreateProjectDeploymentOptions)
	if err != nil {
		return nil, err
	}
	return &CreateProjectDeploymentOutputs{Deployment: deployment}, nil
}
