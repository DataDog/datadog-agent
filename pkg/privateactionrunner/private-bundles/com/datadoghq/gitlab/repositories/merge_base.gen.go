package com_datadoghq_gitlab_repositories

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type MergeBaseHandler struct{}

func NewMergeBaseHandler() *MergeBaseHandler {
	return &MergeBaseHandler{}
}

type MergeBaseInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.MergeBaseOptions
}

type MergeBaseOutputs struct {
	Commit *gitlab.Commit `json:"commit"`
}

func (h *MergeBaseHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[MergeBaseInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commit, _, err := git.Repositories.MergeBase(inputs.ProjectId.String(), inputs.MergeBaseOptions)
	if err != nil {
		return nil, err
	}
	return &MergeBaseOutputs{Commit: commit}, nil
}
