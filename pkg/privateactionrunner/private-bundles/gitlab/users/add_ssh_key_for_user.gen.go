// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type AddSSHKeyForUserHandler struct{}

func NewAddSSHKeyForUserHandler() *AddSSHKeyForUserHandler {
	return &AddSSHKeyForUserHandler{}
}

type AddSSHKeyForUserInputs struct {
	UserId int `json:"user_id,omitempty"`
	*gitlab.AddSSHKeyOptions
}

type AddSSHKeyForUserOutputs struct {
	SshKey *gitlab.SSHKey `json:"ssh_key"`
}

func (h *AddSSHKeyForUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[AddSSHKeyForUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	sshKey, _, err := git.Users.AddSSHKeyForUser(inputs.UserId, inputs.AddSSHKeyOptions)
	if err != nil {
		return nil, err
	}
	return &AddSSHKeyForUserOutputs{SshKey: sshKey}, nil
}
