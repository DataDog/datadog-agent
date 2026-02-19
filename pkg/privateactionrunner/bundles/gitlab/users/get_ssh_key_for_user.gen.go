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

type GetSSHKeyForUserHandler struct{}

func NewGetSSHKeyForUserHandler() *GetSSHKeyForUserHandler {
	return &GetSSHKeyForUserHandler{}
}

type GetSSHKeyForUserInputs struct {
	UserId int64 `json:"user_id,omitempty"`
	KeyId  int64 `json:"key_id,omitempty"`
}

type GetSSHKeyForUserOutputs struct {
	SshKey *gitlab.SSHKey `json:"ssh_key"`
}

func (h *GetSSHKeyForUserHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetSSHKeyForUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	sshKey, _, err := git.Users.GetSSHKeyForUser(inputs.UserId, inputs.KeyId)
	if err != nil {
		return nil, err
	}
	return &GetSSHKeyForUserOutputs{SshKey: sshKey}, nil
}
