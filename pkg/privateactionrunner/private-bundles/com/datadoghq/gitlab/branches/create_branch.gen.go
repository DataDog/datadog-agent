// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_branches

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateBranchHandler struct{}

func NewCreateBranchHandler() *CreateBranchHandler {
	return &CreateBranchHandler{}
}

type CreateBranchInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.CreateBranchOptions
}

type CreateBranchOutputs struct {
	Branch *gitlab.Branch `json:"branch"`
}

func (h *CreateBranchHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateBranchInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	branch, _, err := git.Branches.CreateBranch(inputs.ProjectId.String(), inputs.CreateBranchOptions)
	if err != nil {
		return nil, err
	}
	return &CreateBranchOutputs{Branch: branch}, nil
}
