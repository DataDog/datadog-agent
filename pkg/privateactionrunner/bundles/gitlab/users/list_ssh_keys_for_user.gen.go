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

type ListSSHKeysForUserHandler struct{}

func NewListSSHKeysForUserHandler() *ListSSHKeysForUserHandler {
	return &ListSSHKeysForUserHandler{}
}

type ListSSHKeysForUserInputs struct {
	UserId lib.GitlabID `json:"user_id,omitempty"`
	*gitlab.ListSSHKeysForUserOptions
}

type ListSSHKeysForUserOutputs struct {
	SshKeys []*gitlab.SSHKey `json:"ssh_keys"`
}

func (h *ListSSHKeysForUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListSSHKeysForUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	sshKeys, _, err := git.Users.ListSSHKeysForUser(inputs.UserId.String(), inputs.ListSSHKeysForUserOptions)
	if err != nil {
		return nil, err
	}
	return &ListSSHKeysForUserOutputs{SshKeys: sshKeys}, nil
}
