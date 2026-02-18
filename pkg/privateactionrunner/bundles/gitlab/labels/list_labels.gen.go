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

type ListLabelsHandler struct{}

func NewListLabelsHandler() *ListLabelsHandler {
	return &ListLabelsHandler{}
}

type ListLabelsInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListLabelsOptions
}

type ListLabelsOutputs struct {
	Labels []*gitlab.Label `json:"labels"`
}

func (h *ListLabelsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListLabelsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	labels, _, err := git.Labels.ListLabels(inputs.ProjectId.String(), inputs.ListLabelsOptions)
	if err != nil {
		return nil, err
	}
	return &ListLabelsOutputs{Labels: labels}, nil
}
