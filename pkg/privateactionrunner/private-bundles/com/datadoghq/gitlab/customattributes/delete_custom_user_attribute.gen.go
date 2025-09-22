package com_datadoghq_gitlab_custom_attributes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteCustomUserAttributeHandler struct{}

func NewDeleteCustomUserAttributeHandler() *DeleteCustomUserAttributeHandler {
	return &DeleteCustomUserAttributeHandler{}
}

type DeleteCustomUserAttributeInputs struct {
	UserId int    `json:"user_id,omitempty"`
	Key    string `json:"key,omitempty"`
}

type DeleteCustomUserAttributeOutputs struct{}

func (h *DeleteCustomUserAttributeHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteCustomUserAttributeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.CustomAttribute.DeleteCustomUserAttribute(inputs.UserId, inputs.Key)
	if err != nil {
		return nil, err
	}
	return &DeleteCustomUserAttributeOutputs{}, nil
}
