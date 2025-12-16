// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repositories

import (
	"context"
	"encoding/json"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GetBlobHandler struct{}

func NewGetBlobHandler() *GetBlobHandler {
	return &GetBlobHandler{}
}

type GetBlobInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	Sha       string           `json:"sha,omitempty"`
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
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetBlobInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
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
