// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_protected_branches

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabProtectedBranchesBundle struct{}

func NewGitlabProtectedBranches() types.Bundle {
	return &GitlabProtectedBranchesBundle{}
}

func (b *GitlabProtectedBranchesBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "getProtectedBranch":
		return b.RunGetProtectedBranch(ctx, task, credential)
	case "listProtectedBranches":
		return b.RunListProtectedBranches(ctx, task, credential)
	case "protectRepositoryBranch":
		return b.RunProtectRepositoryBranch(ctx, task, credential)
	case "unprotectRepositoryBranch":
		return b.RunUnprotectRepositoryBranch(ctx, task, credential)
	case "updateProtectedBranch":
		return b.RunUpdateProtectedBranch(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (h *GitlabProtectedBranchesBundle) GetAction(actionName string) types.Action {
	return h
}
