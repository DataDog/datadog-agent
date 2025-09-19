package com_datadoghq_gitlab_repositories

import (
	"context"
	"encoding/json"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
)

type GetBlobHandler struct{}

func NewGetBlobHandler() *GetBlobHandler {
	return &GetBlobHandler{}
}

type GetBlobInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
}

type GetBlobOutputs struct {
	Blob *Blob `json:"blob,omitempty"`
}

type Blob struct {
	Size     int    `json:"size,omitempty"`
	Sha      string `json:"sha,omitempty"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

func (h *GetBlobHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *runtimepb.Credential,
) (any, error) {
	inputs, err := types.ExtractInputs[GetBlobInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	bytes, _, err := git.Repositories.Blob(inputs.ProjectId, inputs.Sha)
	if err != nil {
		return nil, err
	}
	var blob Blob
	if err = json.Unmarshal(bytes, &blob); err != nil {
		return nil, err
	}
	return &GetBlobOutputs{Blob: &blob}, nil
}
