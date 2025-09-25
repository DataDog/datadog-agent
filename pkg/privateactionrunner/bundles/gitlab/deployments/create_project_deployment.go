// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_deployments

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type CreateProjectDeploymentInputs struct {
	ProjectID lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.CreateProjectDeploymentOptions
}

type CreateProjectDeploymentOutputs struct {
	Deployment *gitlab.Deployment `json:"deployment"`
}

func (b *GitlabDeploymentsBundle) RunCreateProjectDeployment(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateProjectDeploymentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}
	deployment, _, err := git.Deployments.CreateProjectDeployment(inputs.ProjectID.String(), inputs.CreateProjectDeploymentOptions)
	if err != nil {
		return nil, err
	}
	return &CreateProjectDeploymentOutputs{Deployment: deployment}, nil
}
