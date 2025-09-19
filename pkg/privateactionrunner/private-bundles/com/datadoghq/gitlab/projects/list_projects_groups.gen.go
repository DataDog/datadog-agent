package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListProjectsGroupsHandler struct{}

func NewListProjectsGroupsHandler() *ListProjectsGroupsHandler {
	return &ListProjectsGroupsHandler{}
}

type ListProjectsGroupsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListProjectGroupOptions
}

type ListProjectsGroupsOutputs struct {
	ProjectGroups []*gitlab.ProjectGroup `json:"project_groups"`
}

func (h *ListProjectsGroupsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListProjectsGroupsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	projectGroups, _, err := git.Projects.ListProjectsGroups(inputs.ProjectId.String(), inputs.ListProjectGroupOptions)
	if err != nil {
		return nil, err
	}
	return &ListProjectsGroupsOutputs{ProjectGroups: projectGroups}, nil
}
