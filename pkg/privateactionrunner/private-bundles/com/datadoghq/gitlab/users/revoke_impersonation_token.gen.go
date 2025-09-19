package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RevokeImpersonationTokenHandler struct{}

func NewRevokeImpersonationTokenHandler() *RevokeImpersonationTokenHandler {
	return &RevokeImpersonationTokenHandler{}
}

type RevokeImpersonationTokenInputs struct {
	UserId               int `json:"user_id,omitempty"`
	ImpersonationTokenId int `json:"impersonation_token_id,omitempty"`
}

type RevokeImpersonationTokenOutputs struct{}

func (h *RevokeImpersonationTokenHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[RevokeImpersonationTokenInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Users.RevokeImpersonationToken(inputs.UserId, inputs.ImpersonationTokenId)
	if err != nil {
		return nil, err
	}
	return &RevokeImpersonationTokenOutputs{}, nil
}
