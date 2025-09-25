// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repository_files

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetFileBlameInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	FilePath  string       `json:"file_path,omitempty"`
	*gitlab.GetFileBlameOptions
}

type GetFileBlameOutputs struct {
	FileBlameRanges []*gitlab.FileBlameRange `json:"file_blame_ranges"`
}

func (b *GitlabRepositoryFilesBundle) RunGetFileBlame(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetFileBlameInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	fileBlameRanges, _, err := git.RepositoryFiles.GetFileBlame(inputs.ProjectId.String(), inputs.FilePath, inputs.GetFileBlameOptions)
	if err != nil {
		return nil, err
	}
	return &GetFileBlameOutputs{FileBlameRanges: fileBlameRanges}, nil
}
