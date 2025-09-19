package com_datadoghq_gitlab_repositories

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GetFileArchiveHandler struct{}

func NewGetFileArchiveHandler() *GetFileArchiveHandler {
	return &GetFileArchiveHandler{}
}

type GetFileArchiveInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ArchiveOptions
}

type GetFileArchiveOutputs struct {
	// This will be converted to base64 with json.Marshal.
	// See https://pkg.go.dev/encoding/json#Marshal
	Content  []byte `json:"content"`
	Encoding string `json:"encoding"`
}

func (h *GetFileArchiveHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetFileArchiveInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	bytes, _, err := git.Repositories.Archive(inputs.ProjectId.String(), inputs.ArchiveOptions)
	if err != nil {
		return nil, err
	}
	return &GetFileArchiveOutputs{Content: bytes, Encoding: "base64"}, nil
}
