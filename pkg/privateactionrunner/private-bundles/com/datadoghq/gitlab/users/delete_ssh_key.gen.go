package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteSSHKeyHandler struct{}

func NewDeleteSSHKeyHandler() *DeleteSSHKeyHandler {
	return &DeleteSSHKeyHandler{}
}

type DeleteSSHKeyInputs struct {
	KeyId int `json:"key_id,omitempty"`
}

type DeleteSSHKeyOutputs struct{}

func (h *DeleteSSHKeyHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteSSHKeyInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Users.DeleteSSHKey(inputs.KeyId)
	if err != nil {
		return nil, err
	}
	return &DeleteSSHKeyOutputs{}, nil
}
