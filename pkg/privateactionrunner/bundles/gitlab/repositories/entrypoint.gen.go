// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repositories

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabRepositoriesBundle struct{}

func NewGitlabRepositories() types.Bundle {
	return &GitlabRepositoriesBundle{}
}

func (b *GitlabRepositoriesBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	// Manual actions
	case "contributors":
		return b.RunContributors(ctx, task, credential)
	case "getBlob":
		return b.RunGetBlob(ctx, task, credential)
	case "getFileArchive":
		return b.RunGetFileArchive(ctx, task, credential)
	case "rawBlobContent":
		return b.RunRawBlobContent(ctx, task, credential)
	// Auto-generated actions
	case "addChangelog":
		return b.RunAddChangelog(ctx, task, credential)
	case "compare":
		return b.RunCompare(ctx, task, credential)
	case "generateChangelogData":
		return b.RunGenerateChangelogData(ctx, task, credential)
	case "listTree":
		return b.RunListTree(ctx, task, credential)
	case "mergeBase":
		return b.RunMergeBase(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (h *GitlabRepositoriesBundle) GetAction(actionName string) types.Action {
	return h
}
