// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_merge_requests

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabMergeRequestsBundle struct{}

func NewGitlabMergeRequests() types.Bundle {
	return &GitlabMergeRequestsBundle{}
}

func (b *GitlabMergeRequestsBundle) GetAction(actionName string) types.Action {
	return b
}

func (b *GitlabMergeRequestsBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "approveMergeRequest":
		return b.RunApproveMergeRequest(ctx, task, credential)
	case "unapproveMergeRequest":
		return b.RunUnapproveMergeRequest(ctx, task, credential)
	case "acceptMergeRequest":
		return b.RunAcceptMergeRequest(ctx, task, credential)
	case "addSpentTime":
		return b.RunAddSpentTime(ctx, task, credential)
	case "cancelMergeWhenPipelineSucceeds":
		return b.RunCancelMergeWhenPipelineSucceeds(ctx, task, credential)
	case "createMergeRequest":
		return b.RunCreateMergeRequest(ctx, task, credential)
	case "createMergeRequestPipeline":
		return b.RunCreateMergeRequestPipeline(ctx, task, credential)
	case "createTodo":
		return b.RunCreateTodo(ctx, task, credential)
	case "deleteMergeRequest":
		return b.RunDeleteMergeRequest(ctx, task, credential)
	case "getIssuesClosedOnMerge":
		return b.RunGetIssuesClosedOnMerge(ctx, task, credential)
	case "getMergeRequest":
		return b.RunGetMergeRequest(ctx, task, credential)
	case "getMergeRequestApprovals":
		return b.RunGetMergeRequestApprovals(ctx, task, credential)
	case "getMergeRequestCommits":
		return b.RunGetMergeRequestCommits(ctx, task, credential)
	case "getMergeRequestDiffVersions":
		return b.RunGetMergeRequestDiffVersions(ctx, task, credential)
	case "getMergeRequestParticipants":
		return b.RunGetMergeRequestParticipants(ctx, task, credential)
	case "getMergeRequestReviewers":
		return b.RunGetMergeRequestReviewers(ctx, task, credential)
	case "getSingleMergeRequestDiffVersion":
		return b.RunGetSingleMergeRequestDiffVersion(ctx, task, credential)
	case "getTimeSpent":
		return b.RunGetTimeSpent(ctx, task, credential)
	case "listGroupMergeRequests":
		return b.RunListGroupMergeRequests(ctx, task, credential)
	case "listMergeRequestDiffs":
		return b.RunListMergeRequestDiffs(ctx, task, credential)
	case "listMergeRequestPipelines":
		return b.RunListMergeRequestPipelines(ctx, task, credential)
	case "listMergeRequests":
		return b.RunListMergeRequests(ctx, task, credential)
	case "listProjectMergeRequests":
		return b.RunListProjectMergeRequests(ctx, task, credential)
	case "rebaseMergeRequest":
		return b.RunRebaseMergeRequest(ctx, task, credential)
	case "resetSpentTime":
		return b.RunResetSpentTime(ctx, task, credential)
	case "resetTimeEstimate":
		return b.RunResetTimeEstimate(ctx, task, credential)
	case "setTimeEstimate":
		return b.RunSetTimeEstimate(ctx, task, credential)
	case "subscribeToMergeRequest":
		return b.RunSubscribeToMergeRequest(ctx, task, credential)
	case "unsubscribeFromMergeRequest":
		return b.RunUnsubscribeFromMergeRequest(ctx, task, credential)
	case "updateMergeRequest":
		return b.RunUpdateMergeRequest(ctx, task, credential)
	default:
		return nil, fmt.Errorf("action not found %s", actionName)
	}
}
