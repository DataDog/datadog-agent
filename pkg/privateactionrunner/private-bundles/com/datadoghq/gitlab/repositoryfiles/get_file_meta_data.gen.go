package com_datadoghq_gitlab_repository_files

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetFileMetaDataHandler struct{}

func NewGetFileMetaDataHandler() *GetFileMetaDataHandler {
	return &GetFileMetaDataHandler{}
}

type GetFileMetaDataInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	FilePath  string       `json:"file_path,omitempty"`
	*gitlab.GetFileMetaDataOptions
}

type GetFileMetaDataOutputs struct {
	File *gitlab.File `json:"file"`
}

func (h *GetFileMetaDataHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetFileMetaDataInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	file, _, err := git.RepositoryFiles.GetFileMetaData(inputs.ProjectId.String(), inputs.FilePath, inputs.GetFileMetaDataOptions)
	if err != nil {
		return nil, err
	}
	return &GetFileMetaDataOutputs{File: file}, nil
}
