package com_datadoghq_gitlab_issues

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListGroupIssuesHandler struct{}

func NewListGroupIssuesHandler() *ListGroupIssuesHandler {
	return &ListGroupIssuesHandler{}
}

type ListGroupIssuesInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListGroupIssuesOptions
}

type ListGroupIssuesOutputs struct {
	Issues []*gitlab.Issue `json:"issues"`
}

func (h *ListGroupIssuesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListGroupIssuesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issues, _, err := git.Issues.ListGroupIssues(inputs.ProjectId.String(), inputs.ListGroupIssuesOptions)
	if err != nil {
		return nil, err
	}
	return &ListGroupIssuesOutputs{Issues: issues}, nil
}
