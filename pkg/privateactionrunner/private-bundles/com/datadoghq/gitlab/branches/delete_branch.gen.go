package com_datadoghq_gitlab_branches

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteBranchHandler struct{}

func NewDeleteBranchHandler() *DeleteBranchHandler {
	return &DeleteBranchHandler{}
}

type DeleteBranchInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Branch    string       `json:"branch,omitempty"`
}

type DeleteBranchOutputs struct{}

func (h *DeleteBranchHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteBranchInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Branches.DeleteBranch(inputs.ProjectId.String(), inputs.Branch)
	if err != nil {
		return nil, err
	}
	return &DeleteBranchOutputs{}, nil
}
