package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListMergeRequestsHandler struct{}

func NewListMergeRequestsHandler() *ListMergeRequestsHandler {
	return &ListMergeRequestsHandler{}
}

type ListMergeRequestsInputs struct {
	*gitlab.ListMergeRequestsOptions
}

type ListMergeRequestsOutputs struct {
	BasicMergeRequests []*gitlab.BasicMergeRequest `json:"merge_requests"`
}

func (h *ListMergeRequestsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListMergeRequestsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	basicMergeRequests, _, err := git.MergeRequests.ListMergeRequests(inputs.ListMergeRequestsOptions)
	if err != nil {
		return nil, err
	}
	return &ListMergeRequestsOutputs{BasicMergeRequests: basicMergeRequests}, nil
}
