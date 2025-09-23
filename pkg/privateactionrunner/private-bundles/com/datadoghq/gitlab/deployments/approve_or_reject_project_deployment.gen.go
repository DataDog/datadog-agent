// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_deployments

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ApproveOrRejectProjectDeploymentHandler struct{}

func NewApproveOrRejectProjectDeploymentHandler() *ApproveOrRejectProjectDeploymentHandler {
	return &ApproveOrRejectProjectDeploymentHandler{}
}

type ApproveOrRejectProjectDeploymentInputs struct {
	ProjectId    lib.GitlabID `json:"project_id,omitempty"`
	DeploymentId int          `json:"deployment_id,omitempty"`
	*gitlab.ApproveOrRejectProjectDeploymentOptions
}

type ApproveOrRejectProjectDeploymentOutputs struct{}

func (h *ApproveOrRejectProjectDeploymentHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ApproveOrRejectProjectDeploymentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Deployments.ApproveOrRejectProjectDeployment(inputs.ProjectId.String(), inputs.DeploymentId, inputs.ApproveOrRejectProjectDeploymentOptions)
	if err != nil {
		return nil, err
	}
	return &ApproveOrRejectProjectDeploymentOutputs{}, nil
}
