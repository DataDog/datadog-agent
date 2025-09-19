package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListUserContributedProjectsHandler struct{}

func NewListUserContributedProjectsHandler() *ListUserContributedProjectsHandler {
	return &ListUserContributedProjectsHandler{}
}

type ListUserContributedProjectsInputs struct {
	UserId lib.GitlabID `json:"user_id,omitempty"`
	*gitlab.ListProjectsOptions
}

type ListUserContributedProjectsOutputs struct {
	Projects []*gitlab.Project `json:"projects"`
}

func (h *ListUserContributedProjectsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListUserContributedProjectsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	projects, _, err := git.Projects.ListUserContributedProjects(inputs.UserId.String(), inputs.ListProjectsOptions)
	if err != nil {
		return nil, err
	}
	return &ListUserContributedProjectsOutputs{Projects: projects}, nil
}
