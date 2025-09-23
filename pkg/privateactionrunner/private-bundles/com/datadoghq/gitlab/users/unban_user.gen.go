// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type UnbanUserHandler struct{}

func NewUnbanUserHandler() *UnbanUserHandler {
	return &UnbanUserHandler{}
}

type UnbanUserInputs struct {
	UserId int `json:"user_id,omitempty"`
}

type UnbanUserOutputs struct{}

func (h *UnbanUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UnbanUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	err = git.Users.UnbanUser(inputs.UserId)
	if err != nil {
		return nil, err
	}
	return &UnbanUserOutputs{}, nil
}
