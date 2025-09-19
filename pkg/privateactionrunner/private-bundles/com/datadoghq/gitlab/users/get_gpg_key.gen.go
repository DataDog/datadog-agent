package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetGPGKeyHandler struct{}

func NewGetGPGKeyHandler() *GetGPGKeyHandler {
	return &GetGPGKeyHandler{}
}

type GetGPGKeyInputs struct {
	KeyId int `json:"key_id,omitempty"`
}

type GetGPGKeyOutputs struct {
	GpgKey *gitlab.GPGKey `json:"gpg_key"`
}

func (h *GetGPGKeyHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetGPGKeyInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	gpgKey, _, err := git.Users.GetGPGKey(inputs.KeyId)
	if err != nil {
		return nil, err
	}
	return &GetGPGKeyOutputs{GpgKey: gpgKey}, nil
}
