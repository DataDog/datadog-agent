// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_pipelines

import (
	"context"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type CreatePipelineHandler struct{}

func NewCreatePipelineHandler() *CreatePipelineHandler {
	return &CreatePipelineHandler{}
}

type CreatePipelineInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*CreatePipelineOptions
}

type CreatePipelineOptions struct {
	*gitlab.CreatePipelineOptions
	// Inputs are not included in gitlab.CreatePipelineOptions so we add it manually
	// TODO: remove this after bumping the sdk version
	Inputs map[string]any `json:"inputs,omitempty"`
}

type CreatePipelineOutputs struct {
	Pipeline *gitlab.Pipeline `json:"pipeline"`
}

func (h *CreatePipelineHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[CreatePipelineInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("projects/%s/pipeline", gitlab.PathEscape(inputs.ProjectId.String()))
	req, err := git.NewRequest(http.MethodPost, u, inputs.CreatePipelineOptions, nil)
	if err != nil {
		return nil, err
	}

	pipeline := new(gitlab.Pipeline)
	_, err = git.Do(req, pipeline)
	if err != nil {
		return nil, err
	}

	return &CreatePipelineOutputs{Pipeline: pipeline}, nil
}
