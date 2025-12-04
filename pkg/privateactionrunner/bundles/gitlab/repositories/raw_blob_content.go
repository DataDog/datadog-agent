// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repositories

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RawBlobContentHandler struct{}

func NewRawBlobContentHandler() *RawBlobContentHandler {
	return &RawBlobContentHandler{}
}

type RawBlobContentInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	Sha       string           `json:"sha,omitempty"`
	Decode    bool             `json:"decode,omitempty"`
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
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[RawBlobContentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
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
