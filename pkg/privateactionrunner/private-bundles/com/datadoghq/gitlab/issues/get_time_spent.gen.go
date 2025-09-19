package com_datadoghq_gitlab_issues

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetTimeSpentHandler struct{}

func NewGetTimeSpentHandler() *GetTimeSpentHandler {
	return &GetTimeSpentHandler{}
}

type GetTimeSpentInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	IssueIid  int          `json:"issue_iid,omitempty"`
}

type GetTimeSpentOutputs struct {
	TimeStats *gitlab.TimeStats `json:"time_stats"`
}

func (h *GetTimeSpentHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetTimeSpentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	timeStats, _, err := git.Issues.GetTimeSpent(inputs.ProjectId.String(), inputs.IssueIid)
	if err != nil {
		return nil, err
	}
	return &GetTimeSpentOutputs{TimeStats: timeStats}, nil
}
