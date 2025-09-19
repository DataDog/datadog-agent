package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type UpdateMergeRequestHandler struct{}

func NewUpdateMergeRequestHandler() *UpdateMergeRequestHandler {
	return &UpdateMergeRequestHandler{}
}

type UpdateMergeRequestInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	*gitlab.UpdateMergeRequestOptions
}

type UpdateMergeRequestOutputs struct {
	MergeRequest *gitlab.MergeRequest `json:"merge_request"`
}

func (h *UpdateMergeRequestHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UpdateMergeRequestInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	mergeRequest, _, err := git.MergeRequests.UpdateMergeRequest(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.UpdateMergeRequestOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateMergeRequestOutputs{MergeRequest: mergeRequest}, nil
}
