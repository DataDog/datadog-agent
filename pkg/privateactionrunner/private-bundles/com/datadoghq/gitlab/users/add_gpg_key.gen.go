package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type AddGPGKeyHandler struct{}

func NewAddGPGKeyHandler() *AddGPGKeyHandler {
	return &AddGPGKeyHandler{}
}

type AddGPGKeyInputs struct {
	*gitlab.AddGPGKeyOptions
}

type AddGPGKeyOutputs struct {
	GpgKey *gitlab.GPGKey `json:"gpg_key"`
}

func (h *AddGPGKeyHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[AddGPGKeyInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	gpgKey, _, err := git.Users.AddGPGKey(inputs.AddGPGKeyOptions)
	if err != nil {
		return nil, err
	}
	return &AddGPGKeyOutputs{GpgKey: gpgKey}, nil
}
