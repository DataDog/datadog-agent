package com_datadoghq_gitlab_jobs

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type RetryJobHandler struct{}

func NewRetryJobHandler() *RetryJobHandler {
	return &RetryJobHandler{}
}

type RetryJobInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	JobId     int          `json:"job_id,omitempty"`
}

type RetryJobOutputs struct {
	Job *gitlab.Job `json:"job"`
}

func (h *RetryJobHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[RetryJobInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	job, _, err := git.Jobs.RetryJob(inputs.ProjectId.String(), inputs.JobId)
	if err != nil {
		return nil, err
	}
	return &RetryJobOutputs{Job: job}, nil
}
