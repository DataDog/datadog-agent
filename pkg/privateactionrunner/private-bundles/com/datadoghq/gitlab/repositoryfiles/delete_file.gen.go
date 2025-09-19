package com_datadoghq_gitlab_repository_files

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type DeleteFileHandler struct{}

func NewDeleteFileHandler() *DeleteFileHandler {
	return &DeleteFileHandler{}
}

type DeleteFileInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	FilePath  string       `json:"file_path,omitempty"`
	*gitlab.DeleteFileOptions
}

type DeleteFileOutputs struct{}

func (h *DeleteFileHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteFileInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.RepositoryFiles.DeleteFile(inputs.ProjectId.String(), inputs.FilePath, inputs.DeleteFileOptions)
	if err != nil {
		return nil, err
	}
	return &DeleteFileOutputs{}, nil
}
