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

type ListProtectedBranchesHandler struct{}

func NewListProtectedBranchesHandler() *ListProtectedBranchesHandler {
	return &ListProtectedBranchesHandler{}
}

type ListProtectedBranchesInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListProtectedBranchesOptions
}

type ListProtectedBranchesOutputs struct {
	ProtectedBranches []*gitlab.ProtectedBranch `json:"protected_branches"`
}

func (h *ListProtectedBranchesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListProtectedBranchesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	protectedBranches, _, err := git.ProtectedBranches.ListProtectedBranches(inputs.ProjectId.String(), inputs.ListProtectedBranchesOptions)
	if err != nil {
		return nil, err
	}
	return &ListProtectedBranchesOutputs{ProtectedBranches: protectedBranches}, nil
}
