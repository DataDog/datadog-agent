package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type AddGPGKeyForUserHandler struct{}

func NewAddGPGKeyForUserHandler() *AddGPGKeyForUserHandler {
	return &AddGPGKeyForUserHandler{}
}

type AddGPGKeyForUserInputs struct {
	UserId int `json:"user_id,omitempty"`
	*gitlab.AddGPGKeyOptions
}

type AddGPGKeyForUserOutputs struct {
	GpgKey *gitlab.GPGKey `json:"gpg_key"`
}

func (h *AddGPGKeyForUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[AddGPGKeyForUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	gpgKey, _, err := git.Users.AddGPGKeyForUser(inputs.UserId, inputs.AddGPGKeyOptions)
	if err != nil {
		return nil, err
	}
	return &AddGPGKeyForUserOutputs{GpgKey: gpgKey}, nil
}
