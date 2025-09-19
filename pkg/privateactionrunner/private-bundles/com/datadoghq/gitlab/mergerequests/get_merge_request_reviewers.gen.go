package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetMergeRequestReviewersHandler struct{}

func NewGetMergeRequestReviewersHandler() *GetMergeRequestReviewersHandler {
	return &GetMergeRequestReviewersHandler{}
}

type GetMergeRequestReviewersInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
}

type GetMergeRequestReviewersOutputs struct {
	MergeRequestReviewers []*gitlab.MergeRequestReviewer `json:"merge_request_reviewers"`
}

func (h *GetMergeRequestReviewersHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetMergeRequestReviewersInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	mergeRequestReviewers, _, err := git.MergeRequests.GetMergeRequestReviewers(inputs.ProjectId.String(), inputs.MergeRequestIid)
	if err != nil {
		return nil, err
	}
	return &GetMergeRequestReviewersOutputs{MergeRequestReviewers: mergeRequestReviewers}, nil
}
