package com_datadoghq_gitlab_commits

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListMergeRequestsByCommitHandler struct{}

func NewListMergeRequestsByCommitHandler() *ListMergeRequestsByCommitHandler {
	return &ListMergeRequestsByCommitHandler{}
}

type ListMergeRequestsByCommitInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
}

type ListMergeRequestsByCommitOutputs struct {
	BasicMergeRequests []*gitlab.BasicMergeRequest `json:"merge_requests"`
}

func (h *ListMergeRequestsByCommitHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListMergeRequestsByCommitInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	basicMergeRequests, _, err := git.Commits.ListMergeRequestsByCommit(inputs.ProjectId.String(), inputs.Sha)
	if err != nil {
		return nil, err
	}
	return &ListMergeRequestsByCommitOutputs{BasicMergeRequests: basicMergeRequests}, nil
}
