package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ApproveUserHandler struct{}

func NewApproveUserHandler() *ApproveUserHandler {
	return &ApproveUserHandler{}
}

type ApproveUserInputs struct {
	UserId int `json:"user_id,omitempty"`
}

type ApproveUserOutputs struct{}

func (h *ApproveUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ApproveUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	err = git.Users.ApproveUser(inputs.UserId)
	if err != nil {
		return nil, err
	}
	return &ApproveUserOutputs{}, nil
}
