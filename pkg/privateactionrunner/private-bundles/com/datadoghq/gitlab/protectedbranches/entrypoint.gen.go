package com_datadoghq_gitlab_protected_branches

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabProtectedBranchesBundle struct {
	actions map[string]types.Action
}

func NewGitlabProtectedBranches() types.Bundle {
	return &GitlabProtectedBranchesBundle{
		actions: map[string]types.Action{
			// Auto-generated actions
			"getProtectedBranch":        NewGetProtectedBranchHandler(),
			"listProtectedBranches":     NewListProtectedBranchesHandler(),
			"protectRepositoryBranch":   NewProtectRepositoryBranchHandler(),
			"unprotectRepositoryBranch": NewUnprotectRepositoryBranchHandler(),
			"updateProtectedBranch":     NewUpdateProtectedBranchHandler(),
		},
	}
}

func (h *GitlabProtectedBranchesBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
