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

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RestoreProjectInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
}

type RestoreProjectOutputs struct {
	Project *gitlab.Project `json:"project"`
}

func (b *GitlabProjectsBundle) RestoreProject(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[RestoreProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
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
