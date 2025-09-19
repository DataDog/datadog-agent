package com_datadoghq_gitlab_issues

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type UpdateIssueHandler struct{}

func NewUpdateIssueHandler() *UpdateIssueHandler {
	return &UpdateIssueHandler{}
}

type UpdateIssueInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	IssueIid  int          `json:"issue_iid,omitempty"`
	*gitlab.UpdateIssueOptions
}

type UpdateIssueOutputs struct {
	Issue *gitlab.Issue `json:"issue"`
}

func (h *UpdateIssueHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UpdateIssueInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issue, _, err := git.Issues.UpdateIssue(inputs.ProjectId.String(), inputs.IssueIid, inputs.UpdateIssueOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateIssueOutputs{Issue: issue}, nil
}
