// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GetImpersonationTokenHandler struct{}

func NewGetImpersonationTokenHandler() *GetImpersonationTokenHandler {
	return &GetImpersonationTokenHandler{}
}

type GetImpersonationTokenInputs struct {
	UserId               int64 `json:"user_id,omitempty"`
	ImpersonationTokenId int64 `json:"impersonation_token_id,omitempty"`
}

type GetImpersonationTokenOutputs struct {
	ImpersonationToken *gitlab.ImpersonationToken `json:"impersonation_token"`
}

func (h *GetImpersonationTokenHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetImpersonationTokenInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	impersonationToken, _, err := git.Users.GetImpersonationToken(inputs.UserId, inputs.ImpersonationTokenId)
	if err != nil {
		return nil, err
	}
	return &GetImpersonationTokenOutputs{ImpersonationToken: impersonationToken}, nil
}
