package com_datadoghq_gitlab_issues

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateIssueHandler struct{}

func NewCreateIssueHandler() *CreateIssueHandler {
	return &CreateIssueHandler{}
}

type CreateIssueInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.CreateIssueOptions
}

type CreateIssueOutputs struct {
	Issue *gitlab.Issue `json:"issue"`
}

func (h *CreateIssueHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateIssueInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issue, _, err := git.Issues.CreateIssue(inputs.ProjectId.String(), inputs.CreateIssueOptions)
	if err != nil {
		return nil, err
	}
	return &CreateIssueOutputs{Issue: issue}, nil
}
