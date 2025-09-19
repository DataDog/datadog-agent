package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateUserRunnerHandler struct{}

func NewCreateUserRunnerHandler() *CreateUserRunnerHandler {
	return &CreateUserRunnerHandler{}
}

type CreateUserRunnerInputs struct {
	*gitlab.CreateUserRunnerOptions
}

type CreateUserRunnerOutputs struct {
	UserRunner *gitlab.UserRunner `json:"user_runner"`
}

func (h *CreateUserRunnerHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateUserRunnerInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	userRunner, _, err := git.Users.CreateUserRunner(inputs.CreateUserRunnerOptions)
	if err != nil {
		return nil, err
	}
	return &CreateUserRunnerOutputs{UserRunner: userRunner}, nil
}
