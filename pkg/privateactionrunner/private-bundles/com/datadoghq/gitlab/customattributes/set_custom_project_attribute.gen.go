package com_datadoghq_gitlab_custom_attributes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type SetCustomProjectAttributeHandler struct{}

func NewSetCustomProjectAttributeHandler() *SetCustomProjectAttributeHandler {
	return &SetCustomProjectAttributeHandler{}
}

type SetCustomProjectAttributeInputs struct {
	ProjectId int `json:"project_id,omitempty"`
	gitlab.CustomAttribute
}

type SetCustomProjectAttributeOutputs struct {
	CustomAttribute *gitlab.CustomAttribute `json:"custom_attribute"`
}

func (h *SetCustomProjectAttributeHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[SetCustomProjectAttributeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	customAttribute, _, err := git.CustomAttribute.SetCustomProjectAttribute(inputs.ProjectId, inputs.CustomAttribute)
	if err != nil {
		return nil, err
	}
	return &SetCustomProjectAttributeOutputs{CustomAttribute: customAttribute}, nil
}
