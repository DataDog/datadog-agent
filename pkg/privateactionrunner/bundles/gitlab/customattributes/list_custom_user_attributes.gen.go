// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_custom_attributes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListCustomUserAttributesInputs struct {
	UserId  int `json:"user_id,omitempty"`
	Page    int `json:"page,omitempty"`
	PerPage int `json:"per_page,omitempty"`
}

type ListCustomUserAttributesOutputs struct {
	CustomAttributes []*gitlab.CustomAttribute `json:"custom_attributes"`
}

func (b *GitlabCustomAttributesBundle) RunListCustomUserAttributes(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListCustomUserAttributesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	customAttributes, _, err := git.CustomAttribute.ListCustomUserAttributes(inputs.UserId, lib.WithPagination(inputs.Page, inputs.PerPage))
	if err != nil {
		return nil, err
	}
	return &ListCustomUserAttributesOutputs{CustomAttributes: customAttributes}, nil
}
