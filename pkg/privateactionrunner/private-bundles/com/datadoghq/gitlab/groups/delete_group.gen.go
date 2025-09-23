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

type DeleteGroupHandler struct{}

func NewDeleteGroupHandler() *DeleteGroupHandler {
	return &DeleteGroupHandler{}
}

type DeleteGroupInputs struct {
	GroupId lib.GitlabID `json:"group_id,omitempty"`
	*gitlab.DeleteGroupOptions
}

type DeleteGroupOutputs struct{}

func (h *DeleteGroupHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteGroupInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Groups.DeleteGroup(inputs.GroupId.String(), inputs.DeleteGroupOptions)
	if err != nil {
		return nil, err
	}
	return &DeleteGroupOutputs{}, nil
}
