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

type DeleteEmailForUserHandler struct{}

func NewDeleteEmailForUserHandler() *DeleteEmailForUserHandler {
	return &DeleteEmailForUserHandler{}
}

type DeleteEmailForUserInputs struct {
	UserId  int64 `json:"user_id,omitempty"`
	EmailId int64 `json:"email_id,omitempty"`
}

type DeleteEmailForUserOutputs struct{}

func (h *DeleteEmailForUserHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[DeleteEmailForUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Users.DeleteEmailForUser(inputs.UserId, inputs.EmailId)
	if err != nil {
		return nil, err
	}
	return &DeleteEmailForUserOutputs{}, nil
}
