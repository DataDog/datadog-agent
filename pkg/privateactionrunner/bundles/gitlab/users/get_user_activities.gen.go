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

type GetUserActivitiesHandler struct{}

func NewGetUserActivitiesHandler() *GetUserActivitiesHandler {
	return &GetUserActivitiesHandler{}
}

type GetUserActivitiesInputs struct {
	*gitlab.GetUserActivitiesOptions
}

type GetUserActivitiesOutputs struct {
	UserActivities []*gitlab.UserActivity `json:"user_activities"`
}

func (h *GetUserActivitiesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetUserActivitiesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	userActivities, _, err := git.Users.GetUserActivities(inputs.GetUserActivitiesOptions)
	if err != nil {
		return nil, err
	}
	return &GetUserActivitiesOutputs{UserActivities: userActivities}, nil
}
