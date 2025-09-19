package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RejectUserHandler struct{}

func NewRejectUserHandler() *RejectUserHandler {
	return &RejectUserHandler{}
}

type RejectUserInputs struct {
	UserId int `json:"user_id,omitempty"`
}

type RejectUserOutputs struct{}

func (h *RejectUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[RejectUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	err = git.Users.RejectUser(inputs.UserId)
	if err != nil {
		return nil, err
	}
	return &RejectUserOutputs{}, nil
}
