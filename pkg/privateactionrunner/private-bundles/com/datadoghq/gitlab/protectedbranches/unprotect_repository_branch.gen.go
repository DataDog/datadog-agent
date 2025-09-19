package com_datadoghq_gitlab_protected_branches

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type UnprotectRepositoryBranchHandler struct{}

func NewUnprotectRepositoryBranchHandler() *UnprotectRepositoryBranchHandler {
	return &UnprotectRepositoryBranchHandler{}
}

type UnprotectRepositoryBranchInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Branch    string       `json:"branch,omitempty"`
}

type UnprotectRepositoryBranchOutputs struct{}

func (h *UnprotectRepositoryBranchHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UnprotectRepositoryBranchInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.ProtectedBranches.UnprotectRepositoryBranches(inputs.ProjectId.String(), inputs.Branch)
	if err != nil {
		return nil, err
	}
	return &UnprotectRepositoryBranchOutputs{}, nil
}
