package com_datadoghq_gitlab_jobs

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CancelJobHandler struct{}

func NewCancelJobHandler() *CancelJobHandler {
	return &CancelJobHandler{}
}

type CancelJobInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	JobId     int          `json:"job_id,omitempty"`
}

type CancelJobOutputs struct {
	Job *gitlab.Job `json:"job"`
}

func (h *CancelJobHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CancelJobInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	job, _, err := git.Jobs.CancelJob(inputs.ProjectId.String(), inputs.JobId)
	if err != nil {
		return nil, err
	}
	return &CancelJobOutputs{Job: job}, nil
}
