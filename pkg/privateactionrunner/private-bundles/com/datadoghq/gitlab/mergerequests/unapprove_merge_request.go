package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type UnapproveMergeRequestHandler struct{}

func NewUnapproveMergeRequestHandler() *UnapproveMergeRequestHandler {
	return &UnapproveMergeRequestHandler{}
}

type UnapproveMergeRequestInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
}

type UnapproveMergeRequestOutputs struct{}

func (h *UnapproveMergeRequestHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UnapproveMergeRequestInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.MergeRequestApprovals.UnapproveMergeRequest(inputs.ProjectId.String(), inputs.MergeRequestIid)
	if err != nil {
		return nil, err
	}
	return &UnapproveMergeRequestOutputs{}, nil
}
