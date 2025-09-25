// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_protected_branches

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetProtectedBranchInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Name      string       `json:"name,omitempty"`
}

type GetProtectedBranchOutputs struct {
	ProtectedBranch *gitlab.ProtectedBranch `json:"protected_branch"`
}

func (b *GitlabProtectedBranchesBundle) RunGetProtectedBranch(
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
