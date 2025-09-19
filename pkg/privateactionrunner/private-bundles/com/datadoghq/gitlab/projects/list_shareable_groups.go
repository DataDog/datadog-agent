package com_datadoghq_gitlab_projects

import (
	"context"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
)

type ListShareableGroupsHandler struct{}

func NewListShareableGroupsHandler() *ListShareableGroupsHandler {
	return &ListShareableGroupsHandler{}
}

type ListShareableGroupsInputs struct {
	ProjectId lib.GitlabID `json:"project_id"`
	*ListShareableGroupsOptions
}

type ListShareableGroupsOptions struct {
	Search *string `url:"search,omitempty" json:"search,omitempty"`
}

type ListShareableGroupsOutputs struct {
	ProjectGroups []*gitlab.ProjectGroup `json:"project_groups"`
}

func (h *ListShareableGroupsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *runtimepb.Credential,
) (any, error) {
	inputs, err := types.ExtractInputs[ListShareableGroupsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("projects/%s/share_locations", gitlab.PathEscape(inputs.ProjectId.String()))

	req, err := git.NewRequest(http.MethodGet, u, inputs.ListShareableGroupsOptions, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab http request: %w", err)
	}
	var projectGroups []*gitlab.ProjectGroup
	_, err = git.Do(req, &projectGroups)
	if err != nil {
		return nil, err
	}

	return &ListShareableGroupsOutputs{ProjectGroups: projectGroups}, nil
}
