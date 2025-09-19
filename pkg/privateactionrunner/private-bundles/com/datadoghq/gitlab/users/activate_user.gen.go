package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ActivateUserHandler struct{}

func NewActivateUserHandler() *ActivateUserHandler {
	return &ActivateUserHandler{}
}

type ActivateUserInputs struct {
	UserId int `json:"user_id,omitempty"`
}

type ActivateUserOutputs struct{}

func (h *ActivateUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ActivateUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	err = git.Users.ActivateUser(inputs.UserId)
	if err != nil {
		return nil, err
	}
	return &ActivateUserOutputs{}, nil
}
