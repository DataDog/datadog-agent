package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetSSHKeyForUserHandler struct{}

func NewGetSSHKeyForUserHandler() *GetSSHKeyForUserHandler {
	return &GetSSHKeyForUserHandler{}
}

type GetSSHKeyForUserInputs struct {
	UserId int `json:"user_id,omitempty"`
	KeyId  int `json:"key_id,omitempty"`
}

type GetSSHKeyForUserOutputs struct {
	SshKey *gitlab.SSHKey `json:"ssh_key"`
}

func (h *GetSSHKeyForUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetSSHKeyForUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	sshKey, _, err := git.Users.GetSSHKeyForUser(inputs.UserId, inputs.KeyId)
	if err != nil {
		return nil, err
	}
	return &GetSSHKeyForUserOutputs{SshKey: sshKey}, nil
}
