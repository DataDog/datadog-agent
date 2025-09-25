// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_jobs

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteProjectArtifactsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
}

type DeleteProjectArtifactsOutputs struct{}

func (b *GitlabJobsBundle) RunDeleteProjectArtifacts(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteProjectArtifactsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Jobs.DeleteProjectArtifacts(inputs.ProjectId.String())
	if err != nil {
		return nil, err
	}
	return &DeleteProjectArtifactsOutputs{}, nil
}
