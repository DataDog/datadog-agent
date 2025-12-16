// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repository_files

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GetFileHandler struct{}

func NewGetFileHandler() *GetFileHandler {
	return &GetFileHandler{}
}

type GetFileInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	FilePath  string           `json:"file_path,omitempty"`
	*gitlab.GetFileOptions
}

type GetFileOutputs struct {
	File *gitlab.File `json:"file"`
}

func (h *GetFileHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetFileInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	file, _, err := git.RepositoryFiles.GetFile(inputs.ProjectId.String(), inputs.FilePath, inputs.GetFileOptions)
	if err != nil {
		return nil, err
	}
	return &GetFileOutputs{File: file}, nil
}
