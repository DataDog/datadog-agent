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

type ListUsersInputs struct {
	*gitlab.ListUsersOptions
}

type ListUsersOutputs struct {
	Users []*gitlab.User `json:"users"`
}

func (b *GitlabUsersBundle) RunListUsers(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListUsersInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	users, _, err := git.Users.ListUsers(inputs.ListUsersOptions)
	if err != nil {
		return nil, err
	}
	return &ListUsersOutputs{Users: users}, nil
}
