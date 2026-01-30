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
)

type DeleteMergedBranchesHandler struct{}

func NewDeleteMergedBranchesHandler() *DeleteMergedBranchesHandler {
	return &DeleteMergedBranchesHandler{}
}

type DeleteMergedBranchesInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
}

type DeleteMergedBranchesOutputs struct{}

func (h *DeleteMergedBranchesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[DeleteMergedBranchesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Branches.DeleteMergedBranches(inputs.ProjectId.String())
	if err != nil {
		return nil, err
	}
	return &DeleteMergedBranchesOutputs{}, nil
}
