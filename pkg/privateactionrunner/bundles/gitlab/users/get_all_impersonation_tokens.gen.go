// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetAllImpersonationTokensInputs struct {
	UserId int `json:"user_id,omitempty"`
	*gitlab.GetAllImpersonationTokensOptions
}

type GetAllImpersonationTokensOutputs struct {
	ImpersonationTokens []*gitlab.ImpersonationToken `json:"impersonation_tokens"`
}

func (b *GitlabUsersBundle) RunGetAllImpersonationTokens(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetAllImpersonationTokensInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	impersonationTokens, _, err := git.Users.GetAllImpersonationTokens(inputs.UserId, inputs.GetAllImpersonationTokensOptions)
	if err != nil {
		return nil, err
	}
	return &GetAllImpersonationTokensOutputs{ImpersonationTokens: impersonationTokens}, nil
}
