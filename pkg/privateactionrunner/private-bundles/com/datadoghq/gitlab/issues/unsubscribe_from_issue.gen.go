package com_datadoghq_gitlab_issues

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type UnsubscribeFromIssueHandler struct{}

func NewUnsubscribeFromIssueHandler() *UnsubscribeFromIssueHandler {
	return &UnsubscribeFromIssueHandler{}
}

type UnsubscribeFromIssueInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	IssueIid  int          `json:"issue_iid,omitempty"`
}

type UnsubscribeFromIssueOutputs struct {
	Issue *gitlab.Issue `json:"issue"`
}

func (h *UnsubscribeFromIssueHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UnsubscribeFromIssueInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issue, _, err := git.Issues.UnsubscribeFromIssue(inputs.ProjectId.String(), inputs.IssueIid)
	if err != nil {
		return nil, err
	}
	return &UnsubscribeFromIssueOutputs{Issue: issue}, nil
}
