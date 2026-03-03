// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_jobs

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type RetryJobHandler struct{}

func NewRetryJobHandler() *RetryJobHandler {
	return &RetryJobHandler{}
}

type RetryJobInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	JobId     int64            `json:"job_id,omitempty"`
}

type RetryJobOutputs struct {
	Job *gitlab.Job `json:"job"`
}

func (h *RetryJobHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[RetryJobInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	job, _, err := git.Jobs.RetryJob(inputs.ProjectId.String(), inputs.JobId)
	if err != nil {
		return nil, err
	}
	return &RetryJobOutputs{Job: job}, nil
}
