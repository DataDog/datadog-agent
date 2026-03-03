// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_projects

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type TransferProjectHandler struct{}

func NewTransferProjectHandler() *TransferProjectHandler {
	return &TransferProjectHandler{}
}

type TransferProjectInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*gitlab.TransferProjectOptions
}

type TransferProjectOutputs struct {
	Project *gitlab.Project `json:"project"`
}

func (h *TransferProjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[TransferProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	project, _, err := git.Projects.TransferProject(inputs.ProjectId.String(), inputs.TransferProjectOptions)
	if err != nil {
		return nil, err
	}
	return &TransferProjectOutputs{Project: project}, nil
}
