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

type CreateLabelHandler struct{}

func NewCreateLabelHandler() *CreateLabelHandler {
	return &CreateLabelHandler{}
}

type CreateLabelInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*gitlab.CreateLabelOptions
}

type CreateLabelOutputs struct {
	Label *gitlab.Label `json:"label"`
}

func (h *CreateLabelHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[CreateLabelInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	label, _, err := git.Labels.CreateLabel(inputs.ProjectId.String(), inputs.CreateLabelOptions)
	if err != nil {
		return nil, err
	}
	return &CreateLabelOutputs{Label: label}, nil
}
