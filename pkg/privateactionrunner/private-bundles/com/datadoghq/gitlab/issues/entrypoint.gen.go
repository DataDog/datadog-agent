package com_datadoghq_gitlab_issues

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabIssuesBundle struct {
	actions map[string]types.Action
}

func NewGitlabIssues() types.Bundle {
	return &GitlabIssuesBundle{
		actions: map[string]types.Action{
			// Auto-generated actions
			"addSpentTime":                    NewAddSpentTimeHandler(),
			"createIssue":                     NewCreateIssueHandler(),
			"createTodo":                      NewCreateTodoHandler(),
			"deleteIssue":                     NewDeleteIssueHandler(),
			"getIssue":                        NewGetIssueHandler(),
			"getIssueByID":                    NewGetIssueByIDHandler(),
			"getParticipants":                 NewGetParticipantsHandler(),
			"getTimeSpent":                    NewGetTimeSpentHandler(),
			"listGroupIssues":                 NewListGroupIssuesHandler(),
			"listIssues":                      NewListIssuesHandler(),
			"listMergeRequestsClosingIssue":   NewListMergeRequestsClosingIssueHandler(),
			"listMergeRequestsRelatedToIssue": NewListMergeRequestsRelatedToIssueHandler(),
			"listProjectIssues":               NewListProjectIssuesHandler(),
			"moveIssue":                       NewMoveIssueHandler(),
			"reorderIssue":                    NewReorderIssueHandler(),
			"resetSpentTime":                  NewResetSpentTimeHandler(),
			"resetTimeEstimate":               NewResetTimeEstimateHandler(),
			"setTimeEstimate":                 NewSetTimeEstimateHandler(),
			"subscribeToIssue":                NewSubscribeToIssueHandler(),
			"unsubscribeFromIssue":            NewUnsubscribeFromIssueHandler(),
			"updateIssue":                     NewUpdateIssueHandler(),
		},
	}
}

func (h *GitlabIssuesBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
