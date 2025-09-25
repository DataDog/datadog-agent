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

type SubscribeToLabelInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	LabelId   lib.GitlabID `json:"label_id,omitempty"`
}

type SubscribeToLabelOutputs struct {
	Label *gitlab.Label `json:"label"`
}

func (b *GitlabLabelsBundle) RunSubscribeToLabel(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[SubscribeToLabelInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	label, _, err := git.Labels.SubscribeToLabel(inputs.ProjectId.String(), inputs.LabelId.String())
	if err != nil {
		return nil, err
	}
	return &SubscribeToLabelOutputs{Label: label}, nil
}
