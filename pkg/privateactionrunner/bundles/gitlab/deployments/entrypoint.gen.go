// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_deployments

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabDeploymentsBundle struct{}

func NewGitlabDeployments() types.Bundle {
	return &GitlabDeploymentsBundle{}
}

func (b *GitlabDeploymentsBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createProjectDeployment":
		return b.RunCreateProjectDeployment(ctx, task, credential)
	case "approveOrRejectProjectDeployment":
		return b.RunApproveOrRejectProjectDeployment(ctx, task, credential)
	case "deleteProjectDeployment":
		return b.RunDeleteProjectDeployment(ctx, task, credential)
	case "getProjectDeployment":
		return b.RunGetProjectDeployment(ctx, task, credential)
	case "listProjectDeployments":
		return b.RunListProjectDeployments(ctx, task, credential)
	case "updateProjectDeployment":
		return b.RunUpdateProjectDeployment(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (b *GitlabDeploymentsBundle) GetAction(actionName string) types.Action {
	return b
}
