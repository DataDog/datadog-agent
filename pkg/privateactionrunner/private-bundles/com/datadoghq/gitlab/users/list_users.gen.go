package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListUsersHandler struct{}

func NewListUsersHandler() *ListUsersHandler {
	return &ListUsersHandler{}
}

type ListUsersInputs struct {
	*gitlab.ListUsersOptions
}

type ListUsersOutputs struct {
	Users []*gitlab.User `json:"users"`
}

func (h *ListUsersHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListUsersInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	users, _, err := git.Users.ListUsers(inputs.ListUsersOptions)
	if err != nil {
		return nil, err
	}
	return &ListUsersOutputs{Users: users}, nil
}
