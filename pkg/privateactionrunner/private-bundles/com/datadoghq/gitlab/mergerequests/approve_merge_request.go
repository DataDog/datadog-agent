package com_datadoghq_gitlab_merge_requests

import (
	"context"
	"encoding/json"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
)

type ApproveMergeRequestHandler struct{}

func NewApproveMergeRequestHandler() *ApproveMergeRequestHandler {
	return &ApproveMergeRequestHandler{}
}

type ApproveMergeRequestInputs struct {
	ProjectId       json.Number `json:"project_id,omitempty"`
	MergeRequestIid int         `json:"merge_request_iid,omitempty"`
	*gitlab.ApproveMergeRequestOptions
}

type ApproveMergeRequestOutputs struct {
	MergeRequestApprovals *gitlab.MergeRequestApprovals `json:"merge_request_approvals"`
}

func (h *ApproveMergeRequestHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *runtimepb.Credential,
) (any, error) {
	inputs, err := types.ExtractInputs[ApproveMergeRequestInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	mergeRequestApprovals, _, err := git.MergeRequestApprovals.ApproveMergeRequest(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.ApproveMergeRequestOptions)
	if err != nil {
		return nil, err
	}
	return &ApproveMergeRequestOutputs{MergeRequestApprovals: mergeRequestApprovals}, nil
}
