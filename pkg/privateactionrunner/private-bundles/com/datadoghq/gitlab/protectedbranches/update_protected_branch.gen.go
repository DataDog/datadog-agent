package com_datadoghq_gitlab_protected_branches

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type UpdateProtectedBranchHandler struct{}

func NewUpdateProtectedBranchHandler() *UpdateProtectedBranchHandler {
	return &UpdateProtectedBranchHandler{}
}

type UpdateProtectedBranchInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Name      string       `json:"name,omitempty"`
	*gitlab.UpdateProtectedBranchOptions
}

type UpdateProtectedBranchOutputs struct {
	ProtectedBranch *gitlab.ProtectedBranch `json:"protected_branch"`
}

func (h *UpdateProtectedBranchHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UpdateProtectedBranchInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	protectedBranch, _, err := git.ProtectedBranches.UpdateProtectedBranch(inputs.ProjectId.String(), inputs.Name, inputs.UpdateProtectedBranchOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateProtectedBranchOutputs{ProtectedBranch: protectedBranch}, nil
}
