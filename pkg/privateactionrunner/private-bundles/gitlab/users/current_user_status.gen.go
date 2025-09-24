// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CurrentUserStatusHandler struct{}

func NewCurrentUserStatusHandler() *CurrentUserStatusHandler {
	return &CurrentUserStatusHandler{}
}

type CurrentUserStatusInputs struct{}

type CurrentUserStatusOutputs struct {
	UserStatus *gitlab.UserStatus `json:"user_status"`
}

func (h *CurrentUserStatusHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	_, err := types.ExtractInputs[CurrentUserStatusInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	userStatus, _, err := git.Users.CurrentUserStatus()
	if err != nil {
		return nil, err
	}
	return &CurrentUserStatusOutputs{UserStatus: userStatus}, nil
}
