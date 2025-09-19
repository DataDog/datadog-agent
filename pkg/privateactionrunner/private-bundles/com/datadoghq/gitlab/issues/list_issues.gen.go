package com_datadoghq_gitlab_issues

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListIssuesHandler struct{}

func NewListIssuesHandler() *ListIssuesHandler {
	return &ListIssuesHandler{}
}

type ListIssuesInputs struct {
	*gitlab.ListIssuesOptions
}

type ListIssuesOutputs struct {
	Issues []*gitlab.Issue `json:"issues"`
}

func (h *ListIssuesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListIssuesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issues, _, err := git.Issues.ListIssues(inputs.ListIssuesOptions)
	if err != nil {
		return nil, err
	}
	return &ListIssuesOutputs{Issues: issues}, nil
}
