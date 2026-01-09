// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_members

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type ListProjectMembersHandler struct{}

func NewListProjectMembersHandler() *ListProjectMembersHandler {
	return &ListProjectMembersHandler{}
}

type ListProjectMembersInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListProjectMembersOptions
}

type ListProjectMembersOutputs struct {
	ProjectMembers []*gitlab.ProjectMember `json:"project_members"`
}

func (h *ListProjectMembersHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListProjectMembersInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	projectMembers, _, err := git.ProjectMembers.ListProjectMembers(inputs.ProjectId.String(), inputs.ListProjectMembersOptions)
	if err != nil {
		return nil, err
	}
	return &ListProjectMembersOutputs{ProjectMembers: projectMembers}, nil
}
