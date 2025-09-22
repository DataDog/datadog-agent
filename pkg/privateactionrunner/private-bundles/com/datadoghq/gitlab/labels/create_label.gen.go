package com_datadoghq_gitlab_labels

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateLabelHandler struct{}

func NewCreateLabelHandler() *CreateLabelHandler {
	return &CreateLabelHandler{}
}

type CreateLabelInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.CreateLabelOptions
}

type CreateLabelOutputs struct {
	Label *gitlab.Label `json:"label"`
}

func (h *CreateLabelHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateLabelInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	label, _, err := git.Labels.CreateLabel(inputs.ProjectId.String(), inputs.CreateLabelOptions)
	if err != nil {
		return nil, err
	}
	return &CreateLabelOutputs{Label: label}, nil
}
