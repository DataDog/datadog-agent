// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repository_files

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GetRawFileHandler struct{}

func NewGetRawFileHandler() *GetRawFileHandler {
	return &GetRawFileHandler{}
}

type GetRawFileInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	FilePath  string           `json:"file_path,omitempty"`
	Encoding  string           `json:"encoding,omitempty"`
	*gitlab.GetRawFileOptions
}

type GetRawFileOutputs struct {
	// []byte or string
	// If []byte, it will be converted to base64 string with json.Marshal.
	// See https://pkg.go.dev/encoding/json#Marshal
	Content any `json:"content,omitempty"`
}

func (h *GetRawFileHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetRawFileInputs](task)
	if err != nil {
		return nil, err
	}
	encoding := "utf-8"
	if inputs.Encoding != "" {
		encoding = inputs.Encoding
	}
	if encoding != "utf-8" && encoding != "base64" {
		return nil, fmt.Errorf("unsupported encoding: %s", encoding)
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	bytes, _, err := git.RepositoryFiles.GetRawFile(inputs.ProjectId.String(), inputs.FilePath, inputs.GetRawFileOptions)
	if err != nil {
		return nil, err
	}
	if encoding == "utf-8" {
		return &GetRawFileOutputs{Content: string(bytes)}, nil
	}
	return &GetRawFileOutputs{Content: bytes}, nil
}
