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

type ListServiceAccountsHandler struct{}

func NewListServiceAccountsHandler() *ListServiceAccountsHandler {
	return &ListServiceAccountsHandler{}
}

type ListServiceAccountsInputs struct {
	*gitlab.ListServiceAccountsOptions
}

type ListServiceAccountsOutputs struct {
	ServiceAccounts []*gitlab.ServiceAccount `json:"service_accounts"`
}

func (h *ListServiceAccountsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListServiceAccountsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	serviceAccounts, _, err := git.Users.ListServiceAccounts(inputs.ListServiceAccountsOptions)
	if err != nil {
		return nil, err
	}
	return &ListServiceAccountsOutputs{ServiceAccounts: serviceAccounts}, nil
}
