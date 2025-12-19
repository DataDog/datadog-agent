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

type GetUserMembershipsHandler struct{}

func NewGetUserMembershipsHandler() *GetUserMembershipsHandler {
	return &GetUserMembershipsHandler{}
}

type GetUserMembershipsInputs struct {
	UserId int64 `json:"user_id,omitempty"`
	*gitlab.GetUserMembershipOptions
}

type GetUserMembershipsOutputs struct {
	UserMemberships []*gitlab.UserMembership `json:"user_memberships"`
}

func (h *GetUserMembershipsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetUserMembershipsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	userMemberships, _, err := git.Users.GetUserMemberships(inputs.UserId, inputs.GetUserMembershipOptions)
	if err != nil {
		return nil, err
	}
	return &GetUserMembershipsOutputs{UserMemberships: userMemberships}, nil
}
