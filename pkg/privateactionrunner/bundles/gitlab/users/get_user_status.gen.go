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

type GetUserStatusHandler struct{}

func NewGetUserStatusHandler() *GetUserStatusHandler {
	return &GetUserStatusHandler{}
}

type GetUserStatusInputs struct {
	UserId int64 `json:"user_id,omitempty"`
}

type GetUserStatusOutputs struct {
	UserStatus *gitlab.UserStatus `json:"user_status"`
}

func (h *GetUserStatusHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetUserStatusInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	userStatus, _, err := git.Users.GetUserStatus(inputs.UserId)
	if err != nil {
		return nil, err
	}
	return &GetUserStatusOutputs{UserStatus: userStatus}, nil
}
