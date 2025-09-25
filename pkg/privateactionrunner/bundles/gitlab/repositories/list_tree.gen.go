// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repositories

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListTreeInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListTreeOptions
}

type ListTreeOutputs struct {
	TreeNodes []*gitlab.TreeNode `json:"tree_nodes"`
}

func (b *GitlabRepositoriesBundle) RunListTree(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListTreeInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	treeNodes, _, err := git.Repositories.ListTree(inputs.ProjectId.String(), inputs.ListTreeOptions)
	if err != nil {
		return nil, err
	}
	return &ListTreeOutputs{TreeNodes: treeNodes}, nil
}
