package com_datadoghq_gitlab_commits

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetCommitCommentsHandler struct{}

func NewGetCommitCommentsHandler() *GetCommitCommentsHandler {
	return &GetCommitCommentsHandler{}
}

type GetCommitCommentsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
	*gitlab.GetCommitCommentsOptions
}

type GetCommitCommentsOutputs struct {
	CommitComments []*gitlab.CommitComment `json:"commit_comments"`
}

func (h *GetCommitCommentsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetCommitCommentsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commitComments, _, err := git.Commits.GetCommitComments(inputs.ProjectId.String(), inputs.Sha, inputs.GetCommitCommentsOptions)
	if err != nil {
		return nil, err
	}
	return &GetCommitCommentsOutputs{CommitComments: commitComments}, nil
}
