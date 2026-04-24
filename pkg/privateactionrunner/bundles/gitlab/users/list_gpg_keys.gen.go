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

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type ListGPGKeysHandler struct{}

func NewListGPGKeysHandler() *ListGPGKeysHandler {
	return &ListGPGKeysHandler{}
}

type ListGPGKeysInputs struct {
	Page    int `json:"page,omitempty"`
	PerPage int `json:"per_page,omitempty"`
}

type ListGPGKeysOutputs struct {
	GpgKeys []*gitlab.GPGKey `json:"gpg_keys"`
}

func (h *ListGPGKeysHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListGPGKeysInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	gpgKeys, _, err := git.Users.ListGPGKeys(support.WithPagination(inputs.Page, inputs.PerPage))
	if err != nil {
		return nil, err
	}
	return &ListGPGKeysOutputs{GpgKeys: gpgKeys}, nil
}
