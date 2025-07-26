// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_deployments

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteProjectDeploymentHandler struct{}

func NewDeleteProjectDeploymentHandler() *DeleteProjectDeploymentHandler {
	return &DeleteProjectDeploymentHandler{}
}

type DeleteProjectDeploymentInputs struct {
	ProjectId    lib.GitlabID `json:"project_id,omitempty"`
	DeploymentId int          `json:"deployment_id,omitempty"`
}

type DeleteProjectDeploymentOutputs struct{}

func (h *DeleteProjectDeploymentHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteProjectDeploymentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Deployments.DeleteProjectDeployment(inputs.ProjectId.String(), inputs.DeploymentId)
	if err != nil {
		return nil, err
	}
	return &DeleteProjectDeploymentOutputs{}, nil
}
