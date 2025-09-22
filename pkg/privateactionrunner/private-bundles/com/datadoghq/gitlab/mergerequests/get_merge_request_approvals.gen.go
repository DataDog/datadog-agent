package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetMergeRequestApprovalsHandler struct{}

func NewGetMergeRequestApprovalsHandler() *GetMergeRequestApprovalsHandler {
	return &GetMergeRequestApprovalsHandler{}
}

type GetMergeRequestApprovalsInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
}

type GetMergeRequestApprovalsOutputs struct {
	MergeRequestApprovals *gitlab.MergeRequestApprovals `json:"merge_request_approvals"`
}

func (h *GetMergeRequestApprovalsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetMergeRequestApprovalsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	mergeRequestApprovals, _, err := git.MergeRequests.GetMergeRequestApprovals(inputs.ProjectId.String(), inputs.MergeRequestIid)
	if err != nil {
		return nil, err
	}
	return &GetMergeRequestApprovalsOutputs{MergeRequestApprovals: mergeRequestApprovals}, nil
}
