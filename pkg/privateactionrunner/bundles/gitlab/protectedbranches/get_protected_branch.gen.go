// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_protected_branches

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GetProtectedBranchHandler struct{}

func NewGetProtectedBranchHandler() *GetProtectedBranchHandler {
	return &GetProtectedBranchHandler{}
}

type GetProtectedBranchInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	Name      string           `json:"name,omitempty"`
}

type GetProtectedBranchOutputs struct {
	ProtectedBranch *gitlab.ProtectedBranch `json:"protected_branch"`
}

func (h *GetProtectedBranchHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetProtectedBranchInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	protectedBranch, _, err := git.ProtectedBranches.GetProtectedBranch(inputs.ProjectId.String(), inputs.Name)
	if err != nil {
		return nil, err
	}
	return &GetProtectedBranchOutputs{ProtectedBranch: protectedBranch}, nil
}
