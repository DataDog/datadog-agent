package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ResetSpentTimeHandler struct{}

func NewResetSpentTimeHandler() *ResetSpentTimeHandler {
	return &ResetSpentTimeHandler{}
}

type ResetSpentTimeInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
}

type ResetSpentTimeOutputs struct {
	TimeStats *gitlab.TimeStats `json:"time_stats"`
}

func (h *ResetSpentTimeHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ResetSpentTimeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	timeStats, _, err := git.MergeRequests.ResetSpentTime(inputs.ProjectId.String(), inputs.MergeRequestIid)
	if err != nil {
		return nil, err
	}
	return &ResetSpentTimeOutputs{TimeStats: timeStats}, nil
}
