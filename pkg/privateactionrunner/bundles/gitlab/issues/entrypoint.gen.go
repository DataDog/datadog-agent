// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_issues

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabIssuesBundle struct{}

func NewGitlabIssues() types.Bundle {
	return &GitlabIssuesBundle{}
}

func (b *GitlabIssuesBundle) GetAction(actionName string) types.Action {
	return b
}

func (b *GitlabIssuesBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "addSpentTime":
		return b.RunAddSpentTime(ctx, task, credential)
	case "createIssue":
		return b.RunCreateIssue(ctx, task, credential)
	case "createTodo":
		return b.RunCreateTodo(ctx, task, credential)
	case "deleteIssue":
		return b.RunDeleteIssue(ctx, task, credential)
	case "getIssue":
		return b.RunGetIssue(ctx, task, credential)
	case "getIssueByID":
		return b.RunGetIssueByID(ctx, task, credential)
	case "getParticipants":
		return b.RunGetParticipants(ctx, task, credential)
	case "getTimeSpent":
		return b.RunGetTimeSpent(ctx, task, credential)
	case "listGroupIssues":
		return b.RunListGroupIssues(ctx, task, credential)
	case "listIssues":
		return b.RunListIssues(ctx, task, credential)
	case "listMergeRequestsClosingIssue":
		return b.RunListMergeRequestsClosingIssue(ctx, task, credential)
	case "listMergeRequestsRelatedToIssue":
		return b.RunListMergeRequestsRelatedToIssue(ctx, task, credential)
	case "listProjectIssues":
		return b.RunListProjectIssues(ctx, task, credential)
	case "moveIssue":
		return b.RunMoveIssue(ctx, task, credential)
	case "reorderIssue":
		return b.RunReorderIssue(ctx, task, credential)
	case "resetSpentTime":
		return b.RunResetSpentTime(ctx, task, credential)
	case "resetTimeEstimate":
		return b.RunResetTimeEstimate(ctx, task, credential)
	case "setTimeEstimate":
		return b.RunSetTimeEstimate(ctx, task, credential)
	case "subscribeToIssue":
		return b.RunSubscribeToIssue(ctx, task, credential)
	case "unsubscribeFromIssue":
		return b.RunUnsubscribeFromIssue(ctx, task, credential)
	case "updateIssue":
		return b.RunUpdateIssue(ctx, task, credential)
	default:
		return nil, fmt.Errorf("action not found %s", actionName)
	}
}
