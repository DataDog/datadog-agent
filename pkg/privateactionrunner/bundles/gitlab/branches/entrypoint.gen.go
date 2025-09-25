// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_branches

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabBranchesBundle struct {
}

func NewGitlabBranches() types.Bundle {
	return &GitlabBranchesBundle{}
}

func (b *GitlabBranchesBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createBranch":
		return b.RunCreateBranch(ctx, task, credential)
	case "deleteBranch":
		return b.RunDeleteBranch(ctx, task, credential)
	case "deleteMergedBranches":
		return b.RunDeleteMergedBranches(ctx, task, credential)
	case "getBranch":
		return b.RunGetBranch(ctx, task, credential)
	case "listBranches":
		return b.RunListBranches(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (h *GitlabBranchesBundle) GetAction(actionName string) types.Action {
	return h
}
