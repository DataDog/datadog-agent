package com_datadoghq_gitlab_repository_files

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetFileBlameHandler struct{}

func NewGetFileBlameHandler() *GetFileBlameHandler {
	return &GetFileBlameHandler{}
}

type GetFileBlameInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	FilePath  string       `json:"file_path,omitempty"`
	*gitlab.GetFileBlameOptions
}

type GetFileBlameOutputs struct {
	FileBlameRanges []*gitlab.FileBlameRange `json:"file_blame_ranges"`
}

func (h *GetFileBlameHandler) Run(
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
