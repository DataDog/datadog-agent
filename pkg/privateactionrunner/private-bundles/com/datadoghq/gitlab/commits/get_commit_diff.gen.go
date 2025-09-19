package com_datadoghq_gitlab_commits

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetCommitDiffHandler struct{}

func NewGetCommitDiffHandler() *GetCommitDiffHandler {
	return &GetCommitDiffHandler{}
}

type GetCommitDiffInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
	*gitlab.GetCommitDiffOptions
}

type GetCommitDiffOutputs struct {
	Diffs []*gitlab.Diff `json:"diffs"`
}

func (h *GetCommitDiffHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetCommitDiffInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	diffs, _, err := git.Commits.GetCommitDiff(inputs.ProjectId.String(), inputs.Sha, inputs.GetCommitDiffOptions)
	if err != nil {
		return nil, err
	}
	return &GetCommitDiffOutputs{Diffs: diffs}, nil
}
