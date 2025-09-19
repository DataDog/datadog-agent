package com_datadoghq_gitlab_custom_attributes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetCustomUserAttributeHandler struct{}

func NewGetCustomUserAttributeHandler() *GetCustomUserAttributeHandler {
	return &GetCustomUserAttributeHandler{}
}

type GetCustomUserAttributeInputs struct {
	UserId int    `json:"user_id,omitempty"`
	Key    string `json:"key,omitempty"`
}

type GetCustomUserAttributeOutputs struct {
	CustomAttribute *gitlab.CustomAttribute `json:"custom_attribute"`
}

func (h *GetCustomUserAttributeHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetCustomUserAttributeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	customAttribute, _, err := git.CustomAttribute.GetCustomUserAttribute(inputs.UserId, inputs.Key)
	if err != nil {
		return nil, err
	}
	return &GetCustomUserAttributeOutputs{CustomAttribute: customAttribute}, nil
}
