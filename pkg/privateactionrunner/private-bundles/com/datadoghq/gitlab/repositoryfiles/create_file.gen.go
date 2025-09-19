package com_datadoghq_gitlab_repository_files

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateFileHandler struct{}

func NewCreateFileHandler() *CreateFileHandler {
	return &CreateFileHandler{}
}

type CreateFileInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	FilePath  string       `json:"file_path,omitempty"`
	*gitlab.CreateFileOptions
}

type CreateFileOutputs struct {
	FileInfo *gitlab.FileInfo `json:"file_info"`
}

func (h *CreateFileHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateFileInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	fileInfo, _, err := git.RepositoryFiles.CreateFile(inputs.ProjectId.String(), inputs.FilePath, inputs.CreateFileOptions)
	if err != nil {
		return nil, err
	}
	return &CreateFileOutputs{FileInfo: fileInfo}, nil
}
