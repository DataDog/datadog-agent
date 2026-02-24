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

type CurrentUserHandler struct{}

func NewCurrentUserHandler() *CurrentUserHandler {
	return &CurrentUserHandler{}
}

type CurrentUserInputs struct{}

type CurrentUserOutputs struct {
	User *gitlab.User `json:"user"`
}

func (h *CurrentUserHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	_, err := types.ExtractInputs[CurrentUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	user, _, err := git.Users.CurrentUser()
	if err != nil {
		return nil, err
	}
	return &CurrentUserOutputs{User: user}, nil
}
