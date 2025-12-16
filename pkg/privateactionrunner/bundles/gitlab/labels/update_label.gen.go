// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_labels

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type UpdateLabelHandler struct{}

func NewUpdateLabelHandler() *UpdateLabelHandler {
	return &UpdateLabelHandler{}
}

type UpdateLabelInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	LabelId   support.GitlabID `json:"label_id,omitempty"`
	*gitlab.UpdateLabelOptions
}

type UpdateLabelOutputs struct {
	Label *gitlab.Label `json:"label"`
}

func (h *UpdateLabelHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[UpdateLabelInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	label, _, err := git.Labels.UpdateLabel(inputs.ProjectId.String(), inputs.LabelId.String(), inputs.UpdateLabelOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateLabelOutputs{Label: label}, nil
}
