// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteEmailHandler struct{}

func NewDeleteEmailHandler() *DeleteEmailHandler {
	return &DeleteEmailHandler{}
}

type DeleteEmailInputs struct {
	EmailId int `json:"email_id,omitempty"`
}

type DeleteEmailOutputs struct{}

func (h *DeleteEmailHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteEmailInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Users.DeleteEmail(inputs.EmailId)
	if err != nil {
		return nil, err
	}
	return &DeleteEmailOutputs{}, nil
}
