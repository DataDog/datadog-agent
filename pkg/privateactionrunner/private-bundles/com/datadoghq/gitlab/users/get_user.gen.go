package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetUserHandler struct{}

func NewGetUserHandler() *GetUserHandler {
	return &GetUserHandler{}
}

type GetUserInputs struct {
	UserId int `json:"user_id,omitempty"`
	gitlab.GetUsersOptions
}

type GetUserOutputs struct {
	User *gitlab.User `json:"user"`
}

func (h *GetUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	user, _, err := git.Users.GetUser(inputs.UserId, inputs.GetUsersOptions)
	if err != nil {
		return nil, err
	}
	return &GetUserOutputs{User: user}, nil
}
