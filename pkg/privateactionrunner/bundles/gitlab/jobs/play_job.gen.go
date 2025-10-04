// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_jobs

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type PlayJobHandler struct{}

func NewPlayJobHandler() *PlayJobHandler {
	return &PlayJobHandler{}
}

type PlayJobInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	JobId     int          `json:"job_id,omitempty"`
	*gitlab.PlayJobOptions
}

type PlayJobOutputs struct {
	Job *gitlab.Job `json:"job"`
}

func (h *PlayJobHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[PlayJobInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	job, _, err := git.Jobs.PlayJob(inputs.ProjectId.String(), inputs.JobId, inputs.PlayJobOptions)
	if err != nil {
		return nil, err
	}
	return &PlayJobOutputs{Job: job}, nil
}
