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

type GetSSHKeyHandler struct{}

func NewGetSSHKeyHandler() *GetSSHKeyHandler {
	return &GetSSHKeyHandler{}
}

type GetSSHKeyInputs struct {
	KeyId int64 `json:"key_id,omitempty"`
}

type GetSSHKeyOutputs struct {
	SshKey *gitlab.SSHKey `json:"ssh_key"`
}

func (h *GetSSHKeyHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetSSHKeyInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	sshKey, _, err := git.Users.GetSSHKey(inputs.KeyId)
	if err != nil {
		return nil, err
	}
	return &GetSSHKeyOutputs{SshKey: sshKey}, nil
}
