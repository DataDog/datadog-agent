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

type GetSSHKeyInputs struct {
	KeyId int `json:"key_id,omitempty"`
}

type GetSSHKeyOutputs struct {
	SshKey *gitlab.SSHKey `json:"ssh_key"`
}

func (b *GitlabUsersBundle) RunGetSSHKey(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetSSHKeyInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	sshKey, _, err := git.Users.GetSSHKey(inputs.KeyId)
	if err != nil {
		return nil, err
	}
	return &GetSSHKeyOutputs{SshKey: sshKey}, nil
}
