package com_datadoghq_gitlab_labels

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type PromoteLabelHandler struct{}

func NewPromoteLabelHandler() *PromoteLabelHandler {
	return &PromoteLabelHandler{}
}

type PromoteLabelInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	LabelId   lib.GitlabID `json:"label_id,omitempty"`
}

type PromoteLabelOutputs struct{}

func (h *PromoteLabelHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[PromoteLabelInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Labels.PromoteLabel(inputs.ProjectId.String(), inputs.LabelId.String())
	if err != nil {
		return nil, err
	}
	return &PromoteLabelOutputs{}, nil
}
