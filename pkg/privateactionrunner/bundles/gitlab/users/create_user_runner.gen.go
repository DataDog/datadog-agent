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

type CreateUserRunnerHandler struct{}

func NewCreateUserRunnerHandler() *CreateUserRunnerHandler {
	return &CreateUserRunnerHandler{}
}

type CreateUserRunnerInputs struct {
	*gitlab.CreateUserRunnerOptions
}

type CreateUserRunnerOutputs struct {
	UserRunner *gitlab.UserRunner `json:"user_runner"`
}

func (h *CreateUserRunnerHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[CreateUserRunnerInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	userRunner, _, err := git.Users.CreateUserRunner(inputs.CreateUserRunnerOptions)
	if err != nil {
		return nil, err
	}
	return &CreateUserRunnerOutputs{UserRunner: userRunner}, nil
}
