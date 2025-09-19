package com_datadoghq_gitlab_branches

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabBranchesBundle struct {
	actions map[string]types.Action
}

func NewGitlabBranches() types.Bundle {
	return &GitlabBranchesBundle{
		actions: map[string]types.Action{
			// Auto-generated actions
			"createBranch":         NewCreateBranchHandler(),
			"deleteBranch":         NewDeleteBranchHandler(),
			"deleteMergedBranches": NewDeleteMergedBranchesHandler(),
			"getBranch":            NewGetBranchHandler(),
			"listBranches":         NewListBranchesHandler(),
		},
	}
}

func (h *GitlabBranchesBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
