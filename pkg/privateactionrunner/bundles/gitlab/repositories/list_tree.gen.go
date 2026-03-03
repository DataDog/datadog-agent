// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repositories

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type ListTreeHandler struct{}

func NewListTreeHandler() *ListTreeHandler {
	return &ListTreeHandler{}
}

type ListTreeInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListTreeOptions
}

type ListTreeOutputs struct {
	TreeNodes []*gitlab.TreeNode `json:"tree_nodes"`
}

func (h *ListTreeHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListTreeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	treeNodes, _, err := git.Repositories.ListTree(inputs.ProjectId.String(), inputs.ListTreeOptions)
	if err != nil {
		return nil, err
	}
	return &ListTreeOutputs{TreeNodes: treeNodes}, nil
}
