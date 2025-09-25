// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_protected_branches

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type UnprotectRepositoryBranchInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Branch    string       `json:"branch,omitempty"`
}

type UnprotectRepositoryBranchOutputs struct{}

func (b *GitlabProtectedBranchesBundle) RunUnprotectRepositoryBranch(
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
