package com_datadoghq_gitlab_protected_branches

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetProtectedBranchHandler struct{}

func NewGetProtectedBranchHandler() *GetProtectedBranchHandler {
	return &GetProtectedBranchHandler{}
}

type GetProtectedBranchInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Name      string       `json:"name,omitempty"`
}

type GetProtectedBranchOutputs struct {
	ProtectedBranch *gitlab.ProtectedBranch `json:"protected_branch"`
}

func (h *GetProtectedBranchHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetProtectedBranchInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	protectedBranch, _, err := git.ProtectedBranches.GetProtectedBranch(inputs.ProjectId.String(), inputs.Name)
	if err != nil {
		return nil, err
	}
	return &GetProtectedBranchOutputs{ProtectedBranch: protectedBranch}, nil
}
