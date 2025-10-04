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

type GetEmailHandler struct{}

func NewGetEmailHandler() *GetEmailHandler {
	return &GetEmailHandler{}
}

type GetEmailInputs struct {
	EmailId int `json:"email_id,omitempty"`
}

type GetEmailOutputs struct {
	Email *gitlab.Email `json:"email"`
}

func (h *GetEmailHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetEmailInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	email, _, err := git.Users.GetEmail(inputs.EmailId)
	if err != nil {
		return nil, err
	}
	return &GetEmailOutputs{Email: email}, nil
}
