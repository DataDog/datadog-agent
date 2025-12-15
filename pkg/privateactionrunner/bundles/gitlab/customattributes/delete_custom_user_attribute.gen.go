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

type DeleteCustomUserAttributeHandler struct{}

func NewDeleteCustomUserAttributeHandler() *DeleteCustomUserAttributeHandler {
	return &DeleteCustomUserAttributeHandler{}
}

type DeleteCustomUserAttributeInputs struct {
	UserId int64  `json:"user_id,omitempty"`
	Key    string `json:"key,omitempty"`
}

type DeleteCustomUserAttributeOutputs struct{}

func (h *DeleteCustomUserAttributeHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[DeleteCustomUserAttributeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.CustomAttribute.DeleteCustomUserAttribute(inputs.UserId, inputs.Key)
	if err != nil {
		return nil, err
	}
	return &DeleteCustomUserAttributeOutputs{}, nil
}
