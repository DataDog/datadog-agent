// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_groups

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type ListGroupsHandler struct{}

func NewListGroupsHandler() *ListGroupsHandler {
	return &ListGroupsHandler{}
}

type ListGroupsInputs struct {
	*gitlab.ListGroupsOptions
}

type ListGroupsOutputs struct {
	Groups []*gitlab.Group `json:"groups"`
}

func (h *ListGroupsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListGroupsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	groups, _, err := git.Groups.ListGroups(inputs.ListGroupsOptions)
	if err != nil {
		return nil, err
	}
	return &ListGroupsOutputs{Groups: groups}, nil
}
