package com_datadoghq_gitlab_custom_attributes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetCustomProjectAttributeHandler struct{}

func NewGetCustomProjectAttributeHandler() *GetCustomProjectAttributeHandler {
	return &GetCustomProjectAttributeHandler{}
}

type GetCustomProjectAttributeInputs struct {
	ProjectId int    `json:"project_id,omitempty"`
	Key       string `json:"key,omitempty"`
}

type GetCustomProjectAttributeOutputs struct {
	CustomAttribute *gitlab.CustomAttribute `json:"custom_attribute"`
}

func (h *GetCustomProjectAttributeHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetCustomProjectAttributeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	customAttribute, _, err := git.CustomAttribute.GetCustomProjectAttribute(inputs.ProjectId, inputs.Key)
	if err != nil {
		return nil, err
	}
	return &GetCustomProjectAttributeOutputs{CustomAttribute: customAttribute}, nil
}
