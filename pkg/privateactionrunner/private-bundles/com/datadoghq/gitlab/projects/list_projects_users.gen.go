package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListProjectsUsersHandler struct{}

func NewListProjectsUsersHandler() *ListProjectsUsersHandler {
	return &ListProjectsUsersHandler{}
}

type ListProjectsUsersInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListProjectUserOptions
}

type ListProjectsUsersOutputs struct {
	ProjectUsers []*gitlab.ProjectUser `json:"project_users"`
}

func (h *ListProjectsUsersHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListProjectsUsersInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	projectUsers, _, err := git.Projects.ListProjectsUsers(inputs.ProjectId.String(), inputs.ListProjectUserOptions)
	if err != nil {
		return nil, err
	}
	return &ListProjectsUsersOutputs{ProjectUsers: projectUsers}, nil
}
