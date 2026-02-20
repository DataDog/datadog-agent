// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repositories

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GetFileArchiveHandler struct{}

func NewGetFileArchiveHandler() *GetFileArchiveHandler {
	return &GetFileArchiveHandler{}
}

type GetFileArchiveInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
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
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetFileArchiveInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	bytes, _, err := git.Repositories.Archive(inputs.ProjectId.String(), inputs.ArchiveOptions)
	if err != nil {
		return nil, err
	}
	return &GetFileArchiveOutputs{Content: bytes, Encoding: "base64"}, nil
}
