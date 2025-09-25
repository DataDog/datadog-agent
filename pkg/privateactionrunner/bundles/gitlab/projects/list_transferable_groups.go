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

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ListTransferableGroupsInputs struct {
	ProjectId lib.GitlabID `json:"project_id"`
	*ListTransferableGroupsOptions
}

type ListTransferableGroupsOptions struct {
	Search  *string `url:"search,omitempty" json:"search,omitempty"`
	Page    int     `url:"page,omitempty" json:"page,omitempty"`
	PerPage int     `url:"per_page,omitempty" json:"per_page,omitempty"`
}

type ListTransferableGroupsOutputs struct {
	ProjectGroups []*gitlab.ProjectGroup `json:"project_groups"`
}

func (b *GitlabProjectsBundle) ListTransferableGroups(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListTransferableGroupsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("projects/%s/transfer_locations", gitlab.PathEscape(inputs.ProjectId.String()))

	req, err := git.NewRequest(http.MethodGet, u, inputs.ListTransferableGroupsOptions, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab http request: %w", err)
	}
	var projectGroups []*gitlab.ProjectGroup
	_, err = git.Do(req, &projectGroups)
	if err != nil {
		return nil, err
	}

	return &ListTransferableGroupsOutputs{ProjectGroups: projectGroups}, nil
}
