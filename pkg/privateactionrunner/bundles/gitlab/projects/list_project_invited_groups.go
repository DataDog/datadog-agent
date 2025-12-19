// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_projects

import (
	"context"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ListProjectInvitedGroupsHandler struct{}

func NewListProjectInvitedGroupsHandler() *ListProjectInvitedGroupsHandler {
	return &ListProjectInvitedGroupsHandler{}
}

type ListProjectInvitedGroupsInputs struct {
	ProjectId support.GitlabID `json:"project_id"`
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
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListProjectInvitedGroupsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
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
