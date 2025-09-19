package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type AddSpentTimeHandler struct{}

func NewAddSpentTimeHandler() *AddSpentTimeHandler {
	return &AddSpentTimeHandler{}
}

type AddSpentTimeInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	*gitlab.AddSpentTimeOptions
}

type AddSpentTimeOutputs struct {
	TimeStats *gitlab.TimeStats `json:"time_stats"`
}

func (h *AddSpentTimeHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[AddSpentTimeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	timeStats, _, err := git.MergeRequests.AddSpentTime(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.AddSpentTimeOptions)
	if err != nil {
		return nil, err
	}
	return &AddSpentTimeOutputs{TimeStats: timeStats}, nil
}
