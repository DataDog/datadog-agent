// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_branches

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GetBranchHandler struct{}

func NewGetBranchHandler() *GetBranchHandler {
	return &GetBranchHandler{}
}

type GetBranchInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	Branch    string           `json:"branch,omitempty"`
}

type GetBranchOutputs struct {
	Branch *gitlab.Branch `json:"branch"`
}

func (h *GetBranchHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetBranchInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	branch, _, err := git.Branches.GetBranch(inputs.ProjectId.String(), inputs.Branch)
	if err != nil {
		return nil, err
	}
	return &GetBranchOutputs{Branch: branch}, nil
}
