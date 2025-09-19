package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type UnbanUserHandler struct{}

func NewUnbanUserHandler() *UnbanUserHandler {
	return &UnbanUserHandler{}
}

type UnbanUserInputs struct {
	UserId int `json:"user_id,omitempty"`
}

type UnbanUserOutputs struct{}

func (h *UnbanUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UnbanUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	err = git.Users.UnbanUser(inputs.UserId)
	if err != nil {
		return nil, err
	}
	return &UnbanUserOutputs{}, nil
}
