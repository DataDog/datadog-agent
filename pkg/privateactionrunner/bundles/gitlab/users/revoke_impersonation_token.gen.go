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
)

type RevokeImpersonationTokenHandler struct{}

func NewRevokeImpersonationTokenHandler() *RevokeImpersonationTokenHandler {
	return &RevokeImpersonationTokenHandler{}
}

type RevokeImpersonationTokenInputs struct {
	UserId               int64 `json:"user_id,omitempty"`
	ImpersonationTokenId int64 `json:"impersonation_token_id,omitempty"`
}

type RevokeImpersonationTokenOutputs struct{}

func (h *RevokeImpersonationTokenHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[RevokeImpersonationTokenInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Users.RevokeImpersonationToken(inputs.UserId, inputs.ImpersonationTokenId)
	if err != nil {
		return nil, err
	}
	return &RevokeImpersonationTokenOutputs{}, nil
}
