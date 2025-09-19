package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type StartHousekeepingProjectHandler struct{}

func NewStartHousekeepingProjectHandler() *StartHousekeepingProjectHandler {
	return &StartHousekeepingProjectHandler{}
}

type StartHousekeepingProjectInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
}

type StartHousekeepingProjectOutputs struct{}

func (h *StartHousekeepingProjectHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[StartHousekeepingProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Projects.StartHousekeepingProject(inputs.ProjectId.String())
	if err != nil {
		return nil, err
	}
	return &StartHousekeepingProjectOutputs{}, nil
}
