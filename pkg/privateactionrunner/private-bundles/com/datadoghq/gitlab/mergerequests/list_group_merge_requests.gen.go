package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListGroupMergeRequestsHandler struct{}

func NewListGroupMergeRequestsHandler() *ListGroupMergeRequestsHandler {
	return &ListGroupMergeRequestsHandler{}
}

type ListGroupMergeRequestsInputs struct {
	GroupId lib.GitlabID `json:"group_id,omitempty"`
	*gitlab.ListGroupMergeRequestsOptions
}

type ListGroupMergeRequestsOutputs struct {
	BasicMergeRequests []*gitlab.BasicMergeRequest `json:"merge_requests"`
}

func (h *ListGroupMergeRequestsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListGroupMergeRequestsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	basicMergeRequests, _, err := git.MergeRequests.ListGroupMergeRequests(inputs.GroupId.String(), inputs.ListGroupMergeRequestsOptions)
	if err != nil {
		return nil, err
	}
	return &ListGroupMergeRequestsOutputs{BasicMergeRequests: basicMergeRequests}, nil
}
