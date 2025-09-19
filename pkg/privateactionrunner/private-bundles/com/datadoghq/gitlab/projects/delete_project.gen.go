package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type DeleteProjectHandler struct{}

func NewDeleteProjectHandler() *DeleteProjectHandler {
	return &DeleteProjectHandler{}
}

type DeleteProjectInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.DeleteProjectOptions
}

type DeleteProjectOutputs struct{}

func (h *DeleteProjectHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Projects.DeleteProject(inputs.ProjectId.String(), inputs.DeleteProjectOptions)
	if err != nil {
		return nil, err
	}
	return &DeleteProjectOutputs{}, nil
}
