package com_datadoghq_gitlab_protected_branches

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ProtectRepositoryBranchHandler struct{}

func NewProtectRepositoryBranchHandler() *ProtectRepositoryBranchHandler {
	return &ProtectRepositoryBranchHandler{}
}

type ProtectRepositoryBranchInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ProtectRepositoryBranchesOptions
}

type ProtectRepositoryBranchOutputs struct {
	ProtectedBranch *gitlab.ProtectedBranch `json:"protected_branch"`
}

func (h *ProtectRepositoryBranchHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ProtectRepositoryBranchInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	protectedBranch, _, err := git.ProtectedBranches.ProtectRepositoryBranches(inputs.ProjectId.String(), inputs.ProtectRepositoryBranchesOptions)
	if err != nil {
		return nil, err
	}
	return &ProtectRepositoryBranchOutputs{ProtectedBranch: protectedBranch}, nil
}
