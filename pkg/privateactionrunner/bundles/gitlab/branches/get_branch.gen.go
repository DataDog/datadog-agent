// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_branches

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetBranchInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Branch    string       `json:"branch,omitempty"`
}

type GetBranchOutputs struct {
	Branch *gitlab.Branch `json:"branch"`
}

func (b *GitlabBranchesBundle) RunGetBranch(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetBranchInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	branch, _, err := git.Branches.GetBranch(inputs.ProjectId.String(), inputs.Branch)
	if err != nil {
		return nil, err
	}
	return &GetBranchOutputs{Branch: branch}, nil
}
