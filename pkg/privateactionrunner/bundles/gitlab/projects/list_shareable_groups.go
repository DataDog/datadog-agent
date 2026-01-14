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

type ListShareableGroupsHandler struct{}

func NewListShareableGroupsHandler() *ListShareableGroupsHandler {
	return &ListShareableGroupsHandler{}
}

type ListShareableGroupsInputs struct {
	ProjectId support.GitlabID `json:"project_id"`
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
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListShareableGroupsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
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
