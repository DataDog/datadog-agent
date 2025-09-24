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

type AddEmailHandler struct{}

func NewAddEmailHandler() *AddEmailHandler {
	return &AddEmailHandler{}
}

type AddEmailInputs struct {
	*gitlab.AddEmailOptions
}

type AddEmailOutputs struct {
	Email *gitlab.Email `json:"email"`
}

func (h *AddEmailHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[AddEmailInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	email, _, err := git.Users.AddEmail(inputs.AddEmailOptions)
	if err != nil {
		return nil, err
	}
	return &AddEmailOutputs{Email: email}, nil
}
