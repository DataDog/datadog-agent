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

type CompareInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.CompareOptions
}

type CompareOutputs struct {
	Compare *gitlab.Compare `json:"compare"`
}

func (b *GitlabRepositoriesBundle) RunCompare(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CompareInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	compare, _, err := git.Repositories.Compare(inputs.ProjectId.String(), inputs.CompareOptions)
	if err != nil {
		return nil, err
	}
	return &CompareOutputs{Compare: compare}, nil
}
