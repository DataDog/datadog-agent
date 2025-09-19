package com_datadoghq_gitlab_custom_attributes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteCustomProjectAttributeHandler struct{}

func NewDeleteCustomProjectAttributeHandler() *DeleteCustomProjectAttributeHandler {
	return &DeleteCustomProjectAttributeHandler{}
}

type DeleteCustomProjectAttributeInputs struct {
	ProjectId int    `json:"project_id,omitempty"`
	Key       string `json:"key,omitempty"`
}

type DeleteCustomProjectAttributeOutputs struct{}

func (h *DeleteCustomProjectAttributeHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteCustomProjectAttributeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.CustomAttribute.DeleteCustomProjectAttribute(inputs.ProjectId, inputs.Key)
	if err != nil {
		return nil, err
	}
	return &DeleteCustomProjectAttributeOutputs{}, nil
}
