package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type TransferProjectHandler struct{}

func NewTransferProjectHandler() *TransferProjectHandler {
	return &TransferProjectHandler{}
}

type TransferProjectInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.TransferProjectOptions
}

type TransferProjectOutputs struct {
	Project *gitlab.Project `json:"project"`
}

func (h *TransferProjectHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[TransferProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	project, _, err := git.Projects.TransferProject(inputs.ProjectId.String(), inputs.TransferProjectOptions)
	if err != nil {
		return nil, err
	}
	return &TransferProjectOutputs{Project: project}, nil
}
