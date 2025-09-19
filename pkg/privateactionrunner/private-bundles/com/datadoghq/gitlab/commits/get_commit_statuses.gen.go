package com_datadoghq_gitlab_commits

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetCommitStatusesHandler struct{}

func NewGetCommitStatusesHandler() *GetCommitStatusesHandler {
	return &GetCommitStatusesHandler{}
}

type GetCommitStatusesInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
	*gitlab.GetCommitStatusesOptions
}

type GetCommitStatusesOutputs struct {
	CommitStatuses []*gitlab.CommitStatus `json:"commit_statuses"`
}

func (h *GetCommitStatusesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetCommitStatusesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commitStatuses, _, err := git.Commits.GetCommitStatuses(inputs.ProjectId.String(), inputs.Sha, inputs.GetCommitStatusesOptions)
	if err != nil {
		return nil, err
	}
	return &GetCommitStatusesOutputs{CommitStatuses: commitStatuses}, nil
}
