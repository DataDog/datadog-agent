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

type GetGPGKeyHandler struct{}

func NewGetGPGKeyHandler() *GetGPGKeyHandler {
	return &GetGPGKeyHandler{}
}

type GetGPGKeyInputs struct {
	KeyId int64 `json:"key_id,omitempty"`
}

type GetGPGKeyOutputs struct {
	GpgKey *gitlab.GPGKey `json:"gpg_key"`
}

func (h *GetGPGKeyHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetGPGKeyInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	gpgKey, _, err := git.Users.GetGPGKey(inputs.KeyId)
	if err != nil {
		return nil, err
	}
	return &GetGPGKeyOutputs{GpgKey: gpgKey}, nil
}
