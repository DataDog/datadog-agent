package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CurrentUserHandler struct{}

func NewCurrentUserHandler() *CurrentUserHandler {
	return &CurrentUserHandler{}
}

type CurrentUserInputs struct{}

type CurrentUserOutputs struct {
	User *gitlab.User `json:"user"`
}

func (h *CurrentUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	_, err := types.ExtractInputs[CurrentUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	user, _, err := git.Users.CurrentUser()
	if err != nil {
		return nil, err
	}
	return &CurrentUserOutputs{User: user}, nil
}
