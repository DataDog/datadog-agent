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
)

type DeleteCustomGroupAttributeHandler struct{}

func NewDeleteCustomGroupAttributeHandler() *DeleteCustomGroupAttributeHandler {
	return &DeleteCustomGroupAttributeHandler{}
}

type DeleteCustomGroupAttributeInputs struct {
	GroupId int64  `json:"group_id,omitempty"`
	Key     string `json:"key,omitempty"`
}

type DeleteCustomGroupAttributeOutputs struct{}

func (h *DeleteCustomGroupAttributeHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[DeleteCustomGroupAttributeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.CustomAttribute.DeleteCustomGroupAttribute(inputs.GroupId, inputs.Key)
	if err != nil {
		return nil, err
	}
	return &DeleteCustomGroupAttributeOutputs{}, nil
}
