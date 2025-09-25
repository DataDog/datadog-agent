// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_commits

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabCommitsBundle struct{}

func NewGitlabCommits() types.Bundle {
	return &GitlabCommitsBundle{}
}

func (b *GitlabCommitsBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "revertCommit":
		return b.RunRevertCommit(ctx, task, credential)
	case "cherryPickCommit":
		return b.RunCherryPickCommit(ctx, task, credential)
	case "createCommit":
		return b.RunCreateCommit(ctx, task, credential)
	case "getCommit":
		return b.RunGetCommit(ctx, task, credential)
	case "getCommitComments":
		return b.RunGetCommitComments(ctx, task, credential)
	case "getCommitDiff":
		return b.RunGetCommitDiff(ctx, task, credential)
	case "getCommitRefs":
		return b.RunGetCommitRefs(ctx, task, credential)
	case "getCommitStatuses":
		return b.RunGetCommitStatuses(ctx, task, credential)
	case "getGPGSignature":
		return b.RunGetGPGSignature(ctx, task, credential)
	case "listCommits":
		return b.RunListCommits(ctx, task, credential)
	case "listMergeRequestsByCommit":
		return b.RunListMergeRequestsByCommit(ctx, task, credential)
	case "postCommitComment":
		return b.RunPostCommitComment(ctx, task, credential)
	case "setCommitStatus":
		return b.RunSetCommitStatus(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (b *GitlabCommitsBundle) GetAction(actionName string) types.Action {
	return b
}
