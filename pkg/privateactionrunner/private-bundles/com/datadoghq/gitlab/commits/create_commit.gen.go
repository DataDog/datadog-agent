package com_datadoghq_gitlab_commits

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateCommitHandler struct{}

func NewCreateCommitHandler() *CreateCommitHandler {
	return &CreateCommitHandler{}
}

type CreateCommitInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.CreateCommitOptions
}

type CreateCommitOutputs struct {
	Commit *gitlab.Commit `json:"commit"`
}

func (h *CreateCommitHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateCommitInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commit, _, err := git.Commits.CreateCommit(inputs.ProjectId.String(), inputs.CreateCommitOptions)
	if err != nil {
		return nil, err
	}
	return &CreateCommitOutputs{Commit: commit}, nil
}
