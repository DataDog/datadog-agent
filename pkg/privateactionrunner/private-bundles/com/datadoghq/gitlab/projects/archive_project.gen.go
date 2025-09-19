package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ArchiveProjectHandler struct{}

func NewArchiveProjectHandler() *ArchiveProjectHandler {
	return &ArchiveProjectHandler{}
}

type ArchiveProjectInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
}

type ArchiveProjectOutputs struct {
	Project *gitlab.Project `json:"project"`
}

func (h *ArchiveProjectHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ArchiveProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	project, _, err := git.Projects.ArchiveProject(inputs.ProjectId.String())
	if err != nil {
		return nil, err
	}
	return &ArchiveProjectOutputs{Project: project}, nil
}
