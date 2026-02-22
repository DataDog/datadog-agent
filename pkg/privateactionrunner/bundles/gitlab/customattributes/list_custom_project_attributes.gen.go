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

type ListCustomProjectAttributesHandler struct{}

func NewListCustomProjectAttributesHandler() *ListCustomProjectAttributesHandler {
	return &ListCustomProjectAttributesHandler{}
}

type ListCustomProjectAttributesInputs struct {
	ProjectId int64 `json:"project_id,omitempty"`
	Page      int   `json:"page,omitempty"`
	PerPage   int   `json:"per_page,omitempty"`
}

type ListCustomProjectAttributesOutputs struct {
	CustomAttributes []*gitlab.CustomAttribute `json:"custom_attributes"`
}

func (h *ListCustomProjectAttributesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListCustomProjectAttributesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	customAttributes, _, err := git.CustomAttribute.ListCustomProjectAttributes(inputs.ProjectId, support.WithPagination(inputs.Page, inputs.PerPage))
	if err != nil {
		return nil, err
	}
	return &ListCustomProjectAttributesOutputs{CustomAttributes: customAttributes}, nil
}
