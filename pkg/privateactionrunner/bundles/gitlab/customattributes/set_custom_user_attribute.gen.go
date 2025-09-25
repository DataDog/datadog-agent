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

type SetCustomUserAttributeInputs struct {
	UserId int `json:"user_id,omitempty"`
	gitlab.CustomAttribute
}

type SetCustomUserAttributeOutputs struct {
	CustomAttribute *gitlab.CustomAttribute `json:"custom_attribute"`
}

func (b *GitlabCustomAttributesBundle) RunSetCustomUserAttribute(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[SetCustomUserAttributeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	customAttribute, _, err := git.CustomAttribute.SetCustomUserAttribute(inputs.UserId, inputs.CustomAttribute)
	if err != nil {
		return nil, err
	}
	return &SetCustomUserAttributeOutputs{CustomAttribute: customAttribute}, nil
}
