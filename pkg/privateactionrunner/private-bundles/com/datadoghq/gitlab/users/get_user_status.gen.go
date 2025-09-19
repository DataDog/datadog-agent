package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetUserStatusHandler struct{}

func NewGetUserStatusHandler() *GetUserStatusHandler {
	return &GetUserStatusHandler{}
}

type GetUserStatusInputs struct {
	UserId int `json:"user_id,omitempty"`
}

type GetUserStatusOutputs struct {
	UserStatus *gitlab.UserStatus `json:"user_status"`
}

func (h *GetUserStatusHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetUserStatusInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	userStatus, _, err := git.Users.GetUserStatus(inputs.UserId)
	if err != nil {
		return nil, err
	}
	return &GetUserStatusOutputs{UserStatus: userStatus}, nil
}
