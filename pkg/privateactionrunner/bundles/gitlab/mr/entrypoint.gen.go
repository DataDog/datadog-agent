// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_merge_requests

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabMergeRequestsBundle struct {
	actions map[string]types.Action
}

func NewGitlabMergeRequests() types.Bundle {
	return &GitlabMergeRequestsBundle{
		actions: map[string]types.Action{
			// Manual actions
			"approveMergeRequest":   NewApproveMergeRequestHandler(),
			"unapproveMergeRequest": NewUnapproveMergeRequestHandler(),
			// Auto-generated actions
			"acceptMergeRequest":               NewAcceptMergeRequestHandler(),
			"addSpentTime":                     NewAddSpentTimeHandler(),
			"cancelMergeWhenPipelineSucceeds":  NewCancelMergeWhenPipelineSucceedsHandler(),
			"createMergeRequest":               NewCreateMergeRequestHandler(),
			"createMergeRequestPipeline":       NewCreateMergeRequestPipelineHandler(),
			"createTodo":                       NewCreateTodoHandler(),
			"deleteMergeRequest":               NewDeleteMergeRequestHandler(),
			"getIssuesClosedOnMerge":           NewGetIssuesClosedOnMergeHandler(),
			"getMergeRequest":                  NewGetMergeRequestHandler(),
			"getMergeRequestApprovals":         NewGetMergeRequestApprovalsHandler(),
			"getMergeRequestCommits":           NewGetMergeRequestCommitsHandler(),
			"getMergeRequestDiffVersions":      NewGetMergeRequestDiffVersionsHandler(),
			"getMergeRequestParticipants":      NewGetMergeRequestParticipantsHandler(),
			"getMergeRequestReviewers":         NewGetMergeRequestReviewersHandler(),
			"getSingleMergeRequestDiffVersion": NewGetSingleMergeRequestDiffVersionHandler(),
			"getTimeSpent":                     NewGetTimeSpentHandler(),
			"listGroupMergeRequests":           NewListGroupMergeRequestsHandler(),
			"listMergeRequestDiffs":            NewListMergeRequestDiffsHandler(),
			"listMergeRequestPipelines":        NewListMergeRequestPipelinesHandler(),
			"listMergeRequests":                NewListMergeRequestsHandler(),
			"listProjectMergeRequests":         NewListProjectMergeRequestsHandler(),
			"rebaseMergeRequest":               NewRebaseMergeRequestHandler(),
			"resetSpentTime":                   NewResetSpentTimeHandler(),
			"resetTimeEstimate":                NewResetTimeEstimateHandler(),
			"setTimeEstimate":                  NewSetTimeEstimateHandler(),
			"subscribeToMergeRequest":          NewSubscribeToMergeRequestHandler(),
			"unsubscribeFromMergeRequest":      NewUnsubscribeFromMergeRequestHandler(),
			"updateMergeRequest":               NewUpdateMergeRequestHandler(),
		},
	}
}

func (h *GitlabMergeRequestsBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
