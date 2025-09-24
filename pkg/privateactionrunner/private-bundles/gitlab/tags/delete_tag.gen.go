// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_tags

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteTagHandler struct{}

func NewDeleteTagHandler() *DeleteTagHandler {
	return &DeleteTagHandler{}
}

type DeleteTagInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	TagName   string       `json:"tag_name,omitempty"`
}

type DeleteTagOutputs struct{}

func (h *DeleteTagHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteTagInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Tags.DeleteTag(inputs.ProjectId.String(), inputs.TagName)
	if err != nil {
		return nil, err
	}
	return &DeleteTagOutputs{}, nil
}
