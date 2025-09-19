package com_datadoghq_gitlab_custom_attributes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteCustomGroupAttributeHandler struct{}

func NewDeleteCustomGroupAttributeHandler() *DeleteCustomGroupAttributeHandler {
	return &DeleteCustomGroupAttributeHandler{}
}

type DeleteCustomGroupAttributeInputs struct {
	GroupId int    `json:"group_id,omitempty"`
	Key     string `json:"key,omitempty"`
}

type DeleteCustomGroupAttributeOutputs struct{}

func (h *DeleteCustomGroupAttributeHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteCustomGroupAttributeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.CustomAttribute.DeleteCustomGroupAttribute(inputs.GroupId, inputs.Key)
	if err != nil {
		return nil, err
	}
	return &DeleteCustomGroupAttributeOutputs{}, nil
}
