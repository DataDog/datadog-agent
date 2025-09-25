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

type ListCustomGroupAttributesInputs struct {
	GroupId int `json:"group_id,omitempty"`
	Page    int `json:"page,omitempty"`
	PerPage int `json:"per_page,omitempty"`
}

type ListCustomGroupAttributesOutputs struct {
	CustomAttributes []*gitlab.CustomAttribute `json:"custom_attributes"`
}

func (b *GitlabCustomAttributesBundle) RunListCustomGroupAttributes(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListCustomGroupAttributesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	customAttributes, _, err := git.CustomAttribute.ListCustomGroupAttributes(inputs.GroupId, lib.WithPagination(inputs.Page, inputs.PerPage))
	if err != nil {
		return nil, err
	}
	return &ListCustomGroupAttributesOutputs{CustomAttributes: customAttributes}, nil
}
