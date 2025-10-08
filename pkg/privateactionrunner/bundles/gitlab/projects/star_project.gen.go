// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type StarProjectHandler struct{}

func NewStarProjectHandler() *StarProjectHandler {
	return &StarProjectHandler{}
}

type StarProjectInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
}

type StarProjectOutputs struct {
	Project *gitlab.Project `json:"project"`
}

func (h *StarProjectHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[StarProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	project, _, err := git.Projects.StarProject(inputs.ProjectId.String())
	if err != nil {
		return nil, err
	}
	return &StarProjectOutputs{Project: project}, nil
}
