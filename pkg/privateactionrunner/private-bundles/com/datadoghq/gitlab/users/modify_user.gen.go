package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ModifyUserHandler struct{}

func NewModifyUserHandler() *ModifyUserHandler {
	return &ModifyUserHandler{}
}

type ModifyUserInputs struct {
	UserId int `json:"user_id,omitempty"`
	*gitlab.ModifyUserOptions
}

type ModifyUserOutputs struct {
	User *gitlab.User `json:"user"`
}

func (h *ModifyUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ModifyUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	user, _, err := git.Users.ModifyUser(inputs.UserId, inputs.ModifyUserOptions)
	if err != nil {
		return nil, err
	}
	return &ModifyUserOutputs{User: user}, nil
}
