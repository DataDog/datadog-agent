package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListGPGKeysForUserHandler struct{}

func NewListGPGKeysForUserHandler() *ListGPGKeysForUserHandler {
	return &ListGPGKeysForUserHandler{}
}

type ListGPGKeysForUserInputs struct {
	UserId  int `json:"user_id,omitempty"`
	Page    int `json:"page,omitempty"`
	PerPage int `json:"per_page,omitempty"`
}

type ListGPGKeysForUserOutputs struct {
	GpgKeys []*gitlab.GPGKey `json:"gpg_keys"`
}

func (h *ListGPGKeysForUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListGPGKeysForUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	gpgKeys, _, err := git.Users.ListGPGKeysForUser(inputs.UserId, lib.WithPagination(inputs.Page, inputs.PerPage))
	if err != nil {
		return nil, err
	}
	return &ListGPGKeysForUserOutputs{GpgKeys: gpgKeys}, nil
}
