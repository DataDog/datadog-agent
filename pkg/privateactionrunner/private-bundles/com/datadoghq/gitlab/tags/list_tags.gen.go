// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_tags

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListTagsHandler struct{}

func NewListTagsHandler() *ListTagsHandler {
	return &ListTagsHandler{}
}

type ListTagsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListTagsOptions
}

type ListTagsOutputs struct {
	Tags []*gitlab.Tag `json:"tags"`
}

func (h *ListTagsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListTagsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	tags, _, err := git.Tags.ListTags(inputs.ProjectId.String(), inputs.ListTagsOptions)
	if err != nil {
		return nil, err
	}
	return &ListTagsOutputs{Tags: tags}, nil
}
