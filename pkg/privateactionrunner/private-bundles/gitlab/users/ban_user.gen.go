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

type BanUserHandler struct{}

func NewBanUserHandler() *BanUserHandler {
	return &BanUserHandler{}
}

type BanUserInputs struct {
	UserId int `json:"user_id,omitempty"`
}

type BanUserOutputs struct{}

func (h *BanUserHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[BanUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	err = git.Users.BanUser(inputs.UserId)
	if err != nil {
		return nil, err
	}
	return &BanUserOutputs{}, nil
}
