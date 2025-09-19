package com_datadoghq_gitlab_projects

import (
	"context"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ListProjectInvitedGroupsHandler struct{}

func NewListProjectInvitedGroupsHandler() *ListProjectInvitedGroupsHandler {
	return &ListProjectInvitedGroupsHandler{}
}

type ListProjectInvitedGroupsInputs struct {
	ProjectId lib.GitlabID `json:"project_id"`
	*ListProjectInvitedGroupsOptions
}

type ListProjectInvitedGroupsOptions struct {
	Search               *string                  `url:"search,omitempty" json:"search,omitempty"`
	MinAccessLevel       *gitlab.AccessLevelValue `url:"min_access_level,omitempty" json:"min_access_level,omitempty"`
	Relation             *[]string                `url:"relation,omitempty" json:"relation,omitempty"`
	WithCustomAttributes *bool                    `url:"with_custom_attributes,omitempty" json:"with_custom_attributes,omitempty"`
}

type ListProjectInvitedGroupsOutputs struct {
	ProjectGroups []*gitlab.ProjectGroup `json:"project_groups"`
}

func (h *ListProjectInvitedGroupsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListProjectInvitedGroupsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("projects/%s/invited_groups", gitlab.PathEscape(inputs.ProjectId.String()))

	req, err := git.NewRequest(http.MethodGet, u, inputs.ListProjectInvitedGroupsOptions, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab http request: %w", err)
	}
	var projectGroups []*gitlab.ProjectGroup
	_, err = git.Do(req, &projectGroups)
	if err != nil {
		return nil, err
	}

	return &ListProjectInvitedGroupsOutputs{ProjectGroups: projectGroups}, nil
}
