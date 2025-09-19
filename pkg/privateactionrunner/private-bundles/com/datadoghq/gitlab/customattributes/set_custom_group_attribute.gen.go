package com_datadoghq_gitlab_custom_attributes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type SetCustomGroupAttributeHandler struct{}

func NewSetCustomGroupAttributeHandler() *SetCustomGroupAttributeHandler {
	return &SetCustomGroupAttributeHandler{}
}

type SetCustomGroupAttributeInputs struct {
	GroupId int `json:"group_id,omitempty"`
	gitlab.CustomAttribute
}

type SetCustomGroupAttributeOutputs struct {
	CustomAttribute *gitlab.CustomAttribute `json:"custom_attribute"`
}

func (h *SetCustomGroupAttributeHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[SetCustomGroupAttributeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	customAttribute, _, err := git.CustomAttribute.SetCustomGroupAttribute(inputs.GroupId, inputs.CustomAttribute)
	if err != nil {
		return nil, err
	}
	return &SetCustomGroupAttributeOutputs{CustomAttribute: customAttribute}, nil
}
