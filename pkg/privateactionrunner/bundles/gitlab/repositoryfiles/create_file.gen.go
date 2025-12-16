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

type CreateFileHandler struct{}

func NewCreateFileHandler() *CreateFileHandler {
	return &CreateFileHandler{}
}

type CreateFileInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	FilePath  string           `json:"file_path,omitempty"`
	*gitlab.CreateFileOptions
}

type CreateFileOutputs struct {
	FileInfo *gitlab.FileInfo `json:"file_info"`
}

func (h *CreateFileHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[CreateFileInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	fileInfo, _, err := git.RepositoryFiles.CreateFile(inputs.ProjectId.String(), inputs.FilePath, inputs.CreateFileOptions)
	if err != nil {
		return nil, err
	}
	return &CreateFileOutputs{FileInfo: fileInfo}, nil
}
