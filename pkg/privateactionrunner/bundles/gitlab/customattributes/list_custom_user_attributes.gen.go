// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_custom_attributes

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type ListCustomUserAttributesHandler struct{}

func NewListCustomUserAttributesHandler() *ListCustomUserAttributesHandler {
	return &ListCustomUserAttributesHandler{}
}

type ListCustomUserAttributesInputs struct {
	UserId  int64 `json:"user_id,omitempty"`
	Page    int   `json:"page,omitempty"`
	PerPage int   `json:"per_page,omitempty"`
}

type ListCustomUserAttributesOutputs struct {
	CustomAttributes []*gitlab.CustomAttribute `json:"custom_attributes"`
}

func (h *ListCustomUserAttributesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListCustomUserAttributesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	customAttributes, _, err := git.CustomAttribute.ListCustomUserAttributes(inputs.UserId, support.WithPagination(inputs.Page, inputs.PerPage))
	if err != nil {
		return nil, err
	}
	return &ListCustomUserAttributesOutputs{CustomAttributes: customAttributes}, nil
}
