// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_tags

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetTagHandler struct{}

func NewGetTagHandler() *GetTagHandler {
	return &GetTagHandler{}
}

type GetTagInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	TagName   string       `json:"tag_name,omitempty"`
}

type GetTagOutputs struct {
	Tag *gitlab.Tag `json:"tag"`
}

func (h *GetTagHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetTagInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	tag, _, err := git.Tags.GetTag(inputs.ProjectId.String(), inputs.TagName)
	if err != nil {
		return nil, err
	}
	return &GetTagOutputs{Tag: tag}, nil
}
