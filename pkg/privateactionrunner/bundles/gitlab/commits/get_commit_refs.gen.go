// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_commits

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetCommitRefsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
	*gitlab.GetCommitRefsOptions
}

type GetCommitRefsOutputs struct {
	CommitRefs []*gitlab.CommitRef `json:"commit_refs"`
}

func (b *GitlabCommitsBundle) RunGetCommitRefs(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetCommitRefsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	commitRefs, _, err := git.Commits.GetCommitRefs(inputs.ProjectId.String(), inputs.Sha, inputs.GetCommitRefsOptions)
	if err != nil {
		return nil, err
	}
	return &GetCommitRefsOutputs{CommitRefs: commitRefs}, nil
}
