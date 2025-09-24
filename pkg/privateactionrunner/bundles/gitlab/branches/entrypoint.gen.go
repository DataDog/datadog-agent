// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

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
