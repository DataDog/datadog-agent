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

type ListEmailsHandler struct{}

func NewListEmailsHandler() *ListEmailsHandler {
	return &ListEmailsHandler{}
}

type ListEmailsInputs struct {
	Page    int `json:"page,omitempty"`
	PerPage int `json:"per_page,omitempty"`
}

type ListEmailsOutputs struct {
	Emails []*gitlab.Email `json:"emails"`
}

func (h *ListEmailsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListEmailsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	emails, _, err := git.Users.ListEmails(support.WithPagination(inputs.Page, inputs.PerPage))
	if err != nil {
		return nil, err
	}
	return &ListEmailsOutputs{Emails: emails}, nil
}
