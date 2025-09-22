package com_datadoghq_gitlab_issues

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type SubscribeToIssueHandler struct{}

func NewSubscribeToIssueHandler() *SubscribeToIssueHandler {
	return &SubscribeToIssueHandler{}
}

type SubscribeToIssueInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	IssueIid  int          `json:"issue_iid,omitempty"`
}

type SubscribeToIssueOutputs struct {
	Issue *gitlab.Issue `json:"issue"`
}

func (h *SubscribeToIssueHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[SubscribeToIssueInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issue, _, err := git.Issues.SubscribeToIssue(inputs.ProjectId.String(), inputs.IssueIid)
	if err != nil {
		return nil, err
	}
	return &SubscribeToIssueOutputs{Issue: issue}, nil
}
