// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_groups

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateGroupHandler struct{}

func NewCreateGroupHandler() *CreateGroupHandler {
	return &CreateGroupHandler{}
}

type CreateGroupInputs struct {
	*gitlab.CreateGroupOptions
}

type CreateGroupOutputs struct {
	Group *gitlab.Group `json:"group"`
}

func (h *CreateGroupHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateGroupInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	group, _, err := git.Groups.CreateGroup(inputs.CreateGroupOptions)
	if err != nil {
		return nil, err
	}
	return &CreateGroupOutputs{Group: group}, nil
}
