package com_datadoghq_gitlab_branches

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListBranchesHandler struct{}

func NewListBranchesHandler() *ListBranchesHandler {
	return &ListBranchesHandler{}
}

type ListBranchesInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListBranchesOptions
}

type ListBranchesOutputs struct {
	Branches []*gitlab.Branch `json:"branches"`
}

func (h *ListBranchesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListBranchesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	branches, _, err := git.Branches.ListBranches(inputs.ProjectId.String(), inputs.ListBranchesOptions)
	if err != nil {
		return nil, err
	}
	return &ListBranchesOutputs{Branches: branches}, nil
}
