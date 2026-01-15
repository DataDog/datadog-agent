// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_projects

import (
	"context"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RestoreProjectHandler struct{}

func NewRestoreProjectHandler() *RestoreProjectHandler {
	return &RestoreProjectHandler{}
}

type RestoreProjectInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
}

type RestoreProjectOutputs struct {
	Project *gitlab.Project `json:"project"`
}

func (h *RestoreProjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[RestoreProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("projects/%s/restore", gitlab.PathEscape(inputs.ProjectId.String()))

	req, err := git.NewRequest(http.MethodPost, u, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab http request: %w", err)
	}
	project := new(gitlab.Project)
	_, err = git.Do(req, &project)
	if err != nil {
		return nil, err
	}

	return &RestoreProjectOutputs{Project: project}, nil
}
