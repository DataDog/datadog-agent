package com_datadoghq_gitlab_commits

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type SetCommitStatusHandler struct{}

func NewSetCommitStatusHandler() *SetCommitStatusHandler {
	return &SetCommitStatusHandler{}
}

type SetCommitStatusInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
	*gitlab.SetCommitStatusOptions
}

type SetCommitStatusOutputs struct {
	CommitStatus *gitlab.CommitStatus `json:"commit_status"`
}

func (h *SetCommitStatusHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[SetCommitStatusInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commitStatus, _, err := git.Commits.SetCommitStatus(inputs.ProjectId.String(), inputs.Sha, inputs.SetCommitStatusOptions)
	if err != nil {
		return nil, err
	}
	return &SetCommitStatusOutputs{CommitStatus: commitStatus}, nil
}
