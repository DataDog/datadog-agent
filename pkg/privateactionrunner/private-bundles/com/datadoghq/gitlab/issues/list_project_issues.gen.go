package com_datadoghq_gitlab_issues

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListProjectIssuesHandler struct{}

func NewListProjectIssuesHandler() *ListProjectIssuesHandler {
	return &ListProjectIssuesHandler{}
}

type ListProjectIssuesInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListProjectIssuesOptions
}

type ListProjectIssuesOutputs struct {
	Issues []*gitlab.Issue `json:"issues"`
}

func (h *ListProjectIssuesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListProjectIssuesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issues, _, err := git.Issues.ListProjectIssues(inputs.ProjectId.String(), inputs.ListProjectIssuesOptions)
	if err != nil {
		return nil, err
	}
	return &ListProjectIssuesOutputs{Issues: issues}, nil
}
