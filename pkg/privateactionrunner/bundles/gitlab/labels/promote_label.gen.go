// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_labels

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type PromoteLabelInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	LabelId   lib.GitlabID `json:"label_id,omitempty"`
}

type PromoteLabelOutputs struct{}

func (b *GitlabLabelsBundle) RunPromoteLabel(
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
