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

type AddSSHKeyHandler struct{}

func NewAddSSHKeyHandler() *AddSSHKeyHandler {
	return &AddSSHKeyHandler{}
}

type AddSSHKeyInputs struct {
	*gitlab.AddSSHKeyOptions
}

type AddSSHKeyOutputs struct {
	SshKey *gitlab.SSHKey `json:"ssh_key"`
}

func (h *AddSSHKeyHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[AddSSHKeyInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	sshKey, _, err := git.Users.AddSSHKey(inputs.AddSSHKeyOptions)
	if err != nil {
		return nil, err
	}
	return &AddSSHKeyOutputs{SshKey: sshKey}, nil
}
