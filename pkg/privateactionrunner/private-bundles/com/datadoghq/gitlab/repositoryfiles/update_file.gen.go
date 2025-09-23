// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repository_files

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type UpdateFileHandler struct{}

func NewUpdateFileHandler() *UpdateFileHandler {
	return &UpdateFileHandler{}
}

type UpdateFileInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	FilePath  string       `json:"file_path,omitempty"`
	*gitlab.UpdateFileOptions
}

type UpdateFileOutputs struct {
	FileInfo *gitlab.FileInfo `json:"file_info"`
}

func (h *UpdateFileHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UpdateFileInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	fileInfo, _, err := git.RepositoryFiles.UpdateFile(inputs.ProjectId.String(), inputs.FilePath, inputs.UpdateFileOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateFileOutputs{FileInfo: fileInfo}, nil
}
