package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListMergeRequestDiffsHandler struct{}

func NewListMergeRequestDiffsHandler() *ListMergeRequestDiffsHandler {
	return &ListMergeRequestDiffsHandler{}
}

type ListMergeRequestDiffsInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	*gitlab.ListMergeRequestDiffsOptions
}

type ListMergeRequestDiffsOutputs struct {
	MergeRequestDiffs []*gitlab.MergeRequestDiff `json:"merge_request_diffs"`
}

func (h *ListMergeRequestDiffsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListMergeRequestDiffsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	mergeRequestDiffs, _, err := git.MergeRequests.ListMergeRequestDiffs(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.ListMergeRequestDiffsOptions)
	if err != nil {
		return nil, err
	}
	return &ListMergeRequestDiffsOutputs{MergeRequestDiffs: mergeRequestDiffs}, nil
}
