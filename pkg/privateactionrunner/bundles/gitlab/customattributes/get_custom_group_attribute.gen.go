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

type GetCustomGroupAttributeInputs struct {
	GroupId int    `json:"group_id,omitempty"`
	Key     string `json:"key,omitempty"`
}

type GetCustomGroupAttributeOutputs struct {
	CustomAttribute *gitlab.CustomAttribute `json:"custom_attribute"`
}

func (b *GitlabCustomAttributesBundle) RunGetCustomGroupAttribute(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetCustomGroupAttributeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	customAttribute, _, err := git.CustomAttribute.GetCustomGroupAttribute(inputs.GroupId, inputs.Key)
	if err != nil {
		return nil, err
	}
	return &GetCustomGroupAttributeOutputs{CustomAttribute: customAttribute}, nil
}
