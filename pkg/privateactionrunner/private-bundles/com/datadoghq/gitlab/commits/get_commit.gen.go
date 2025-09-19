package com_datadoghq_gitlab_commits

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetCommitHandler struct{}

func NewGetCommitHandler() *GetCommitHandler {
	return &GetCommitHandler{}
}

type GetCommitInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
	*gitlab.GetCommitOptions
}

type GetCommitOutputs struct {
	Commit *gitlab.Commit `json:"commit"`
}

func (h *GetCommitHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetCommitInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commit, _, err := git.Commits.GetCommit(inputs.ProjectId.String(), inputs.Sha, inputs.GetCommitOptions)
	if err != nil {
		return nil, err
	}
	return &GetCommitOutputs{Commit: commit}, nil
}
