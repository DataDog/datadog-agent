package com_datadoghq_gitlab_jobs

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetJobTokensJobHandler struct{}

func NewGetJobTokensJobHandler() *GetJobTokensJobHandler {
	return &GetJobTokensJobHandler{}
}

type GetJobTokensJobInputs struct {
	*gitlab.GetJobTokensJobOptions
}

type GetJobTokensJobOutputs struct {
	Job *gitlab.Job `json:"job"`
}

func (h *GetJobTokensJobHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetJobTokensJobInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	job, _, err := git.Jobs.GetJobTokensJob(inputs.GetJobTokensJobOptions)
	if err != nil {
		return nil, err
	}
	return &GetJobTokensJobOutputs{Job: job}, nil
}
