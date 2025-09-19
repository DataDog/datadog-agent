package com_datadoghq_gitlab_deployments

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabDeploymentsBundle struct {
	actions map[string]types.Action
}

func NewGitlabDeployments() types.Bundle {
	return &GitlabDeploymentsBundle{
		actions: map[string]types.Action{
			// Manual actions
			"createProjectDeployment": NewCreateProjectDeploymentHandler(),
			// Auto-generated actions
			"approveOrRejectProjectDeployment": NewApproveOrRejectProjectDeploymentHandler(),
			"deleteProjectDeployment":          NewDeleteProjectDeploymentHandler(),
			"getProjectDeployment":             NewGetProjectDeploymentHandler(),
			"listProjectDeployments":           NewListProjectDeploymentsHandler(),
			"updateProjectDeployment":          NewUpdateProjectDeploymentHandler(),
		},
	}
}

func (h *GitlabDeploymentsBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
