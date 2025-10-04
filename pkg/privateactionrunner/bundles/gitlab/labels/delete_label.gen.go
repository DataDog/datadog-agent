// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_labels

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type DeleteLabelHandler struct{}

func NewDeleteLabelHandler() *DeleteLabelHandler {
	return &DeleteLabelHandler{}
}

type DeleteLabelInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	LabelId   lib.GitlabID `json:"label_id,omitempty"`
	*gitlab.DeleteLabelOptions
}

type DeleteLabelOutputs struct{}

func (h *DeleteLabelHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteLabelInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Labels.DeleteLabel(inputs.ProjectId.String(), inputs.LabelId.String(), inputs.DeleteLabelOptions)
	if err != nil {
		return nil, err
	}
	return &DeleteLabelOutputs{}, nil
}
