package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetSSHKeyHandler struct{}

func NewGetSSHKeyHandler() *GetSSHKeyHandler {
	return &GetSSHKeyHandler{}
}

type GetSSHKeyInputs struct {
	KeyId int `json:"key_id,omitempty"`
}

type GetSSHKeyOutputs struct {
	SshKey *gitlab.SSHKey `json:"ssh_key"`
}

func (h *GetSSHKeyHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetSSHKeyInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	sshKey, _, err := git.Users.GetSSHKey(inputs.KeyId)
	if err != nil {
		return nil, err
	}
	return &GetSSHKeyOutputs{SshKey: sshKey}, nil
}
