// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_labels

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type UnsubscribeFromLabelHandler struct{}

func NewUnsubscribeFromLabelHandler() *UnsubscribeFromLabelHandler {
	return &UnsubscribeFromLabelHandler{}
}

type UnsubscribeFromLabelInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	LabelId   lib.GitlabID `json:"label_id,omitempty"`
}

type UnsubscribeFromLabelOutputs struct{}

func (h *UnsubscribeFromLabelHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UnsubscribeFromLabelInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Labels.UnsubscribeFromLabel(inputs.ProjectId.String(), inputs.LabelId.String())
	if err != nil {
		return nil, err
	}
	return &UnsubscribeFromLabelOutputs{}, nil
}
