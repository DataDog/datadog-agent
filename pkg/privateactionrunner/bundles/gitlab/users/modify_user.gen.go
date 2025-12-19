// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type ModifyUserHandler struct{}

func NewModifyUserHandler() *ModifyUserHandler {
	return &ModifyUserHandler{}
}

type ModifyUserInputs struct {
	UserId int64 `json:"user_id,omitempty"`
	*gitlab.ModifyUserOptions
}

type ModifyUserOutputs struct {
	User *gitlab.User `json:"user"`
}

func (h *ModifyUserHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ModifyUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	user, _, err := git.Users.ModifyUser(inputs.UserId, inputs.ModifyUserOptions)
	if err != nil {
		return nil, err
	}
	return &ModifyUserOutputs{User: user}, nil
}
