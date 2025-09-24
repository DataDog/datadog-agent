// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_tags

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateTagHandler struct{}

func NewCreateTagHandler() *CreateTagHandler {
	return &CreateTagHandler{}
}

type CreateTagInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.CreateTagOptions
}

type CreateTagOutputs struct {
	Tag *gitlab.Tag `json:"tag"`
}

func (h *CreateTagHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateTagInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	tag, _, err := git.Tags.CreateTag(inputs.ProjectId.String(), inputs.CreateTagOptions)
	if err != nil {
		return nil, err
	}
	return &CreateTagOutputs{Tag: tag}, nil
}
