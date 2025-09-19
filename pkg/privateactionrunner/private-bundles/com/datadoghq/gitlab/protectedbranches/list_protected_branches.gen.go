package com_datadoghq_gitlab_protected_branches

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListProtectedBranchesHandler struct{}

func NewListProtectedBranchesHandler() *ListProtectedBranchesHandler {
	return &ListProtectedBranchesHandler{}
}

type ListProtectedBranchesInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListProtectedBranchesOptions
}

type ListProtectedBranchesOutputs struct {
	ProtectedBranches []*gitlab.ProtectedBranch `json:"protected_branches"`
}

func (h *ListProtectedBranchesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListProtectedBranchesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	protectedBranches, _, err := git.ProtectedBranches.ListProtectedBranches(inputs.ProjectId.String(), inputs.ListProtectedBranchesOptions)
	if err != nil {
		return nil, err
	}
	return &ListProtectedBranchesOutputs{ProtectedBranches: protectedBranches}, nil
}
