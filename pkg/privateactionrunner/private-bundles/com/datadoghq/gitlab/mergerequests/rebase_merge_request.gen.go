package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type RebaseMergeRequestHandler struct{}

func NewRebaseMergeRequestHandler() *RebaseMergeRequestHandler {
	return &RebaseMergeRequestHandler{}
}

type RebaseMergeRequestInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	*gitlab.RebaseMergeRequestOptions
}

type RebaseMergeRequestOutputs struct{}

func (h *RebaseMergeRequestHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[RebaseMergeRequestInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.MergeRequests.RebaseMergeRequest(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.RebaseMergeRequestOptions)
	if err != nil {
		return nil, err
	}
	return &RebaseMergeRequestOutputs{}, nil
}
