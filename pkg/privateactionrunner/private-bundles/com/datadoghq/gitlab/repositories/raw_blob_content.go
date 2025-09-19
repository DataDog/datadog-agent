package com_datadoghq_gitlab_repositories

import (
	"context"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
)

type RawBlobContentHandler struct{}

func NewRawBlobContentHandler() *RawBlobContentHandler {
	return &RawBlobContentHandler{}
}

type RawBlobContentInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	Sha       string       `json:"sha,omitempty"`
	Decode    bool         `json:"decode,omitempty"`
}

type RawBlobContentOutputs struct {
	// []byte or string
	// If []byte, it will be converted to base64 string with json.Marshal.
	// See https://pkg.go.dev/encoding/json#Marshal
	Content  any    `json:"content"`
	Encoding string `json:"encoding"`
}

func (h *RawBlobContentHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *runtimepb.Credential,
) (any, error) {
	inputs, err := types.ExtractInputs[RawBlobContentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	bytes, _, err := git.Repositories.RawBlobContent(inputs.ProjectId.String(), inputs.Sha)
	if err != nil {
		return nil, err
	}
	if inputs.Decode {
		return &RawBlobContentOutputs{Content: string(bytes), Encoding: "utf-8"}, nil
	}
	return &RawBlobContentOutputs{Content: bytes, Encoding: "base64"}, nil
}
