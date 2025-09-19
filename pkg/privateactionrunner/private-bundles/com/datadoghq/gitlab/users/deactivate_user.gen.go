package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeactivateUserHandler struct{}

func NewDeactivateUserHandler() *DeactivateUserHandler {
	return &DeactivateUserHandler{}
}

type DeactivateUserInputs struct {
	UserId int `json:"user_id,omitempty"`
}

type DeactivateUserOutputs struct{}

func (h *DeactivateUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeactivateUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	err = git.Users.DeactivateUser(inputs.UserId)
	if err != nil {
		return nil, err
	}
	return &DeactivateUserOutputs{}, nil
}
