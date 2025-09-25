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

type UpdateProjectDeploymentInputs struct {
	ProjectId    lib.GitlabID `json:"project_id,omitempty"`
	DeploymentId int          `json:"deployment_id,omitempty"`
	*gitlab.UpdateProjectDeploymentOptions
}

type UpdateProjectDeploymentOutputs struct {
	Deployment *gitlab.Deployment `json:"deployment"`
}

func (b *GitlabDeploymentsBundle) RunUpdateProjectDeployment(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UpdateProjectDeploymentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	deployment, _, err := git.Deployments.UpdateProjectDeployment(inputs.ProjectId.String(), inputs.DeploymentId, inputs.UpdateProjectDeploymentOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateProjectDeploymentOutputs{Deployment: deployment}, nil
}
