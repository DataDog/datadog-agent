// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateUserHandler struct{}

func NewCreateUserHandler() *CreateUserHandler {
	return &CreateUserHandler{}
}

type CreateUserInputs struct {
	*gitlab.CreateUserOptions
}

type CreateUserOutputs struct {
	User *gitlab.User `json:"user"`
}

func (h *CreateUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	user, _, err := git.Users.CreateUser(inputs.CreateUserOptions)
	if err != nil {
		return nil, err
	}
	return &CreateUserOutputs{User: user}, nil
}
