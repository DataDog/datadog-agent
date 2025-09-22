package com_datadoghq_gitlab_custom_attributes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetCustomGroupAttributeHandler struct{}

func NewGetCustomGroupAttributeHandler() *GetCustomGroupAttributeHandler {
	return &GetCustomGroupAttributeHandler{}
}

type GetCustomGroupAttributeInputs struct {
	GroupId int    `json:"group_id,omitempty"`
	Key     string `json:"key,omitempty"`
}

type GetCustomGroupAttributeOutputs struct {
	CustomAttribute *gitlab.CustomAttribute `json:"custom_attribute"`
}

func (h *GetCustomGroupAttributeHandler) Run(
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
