package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteSharedProjectFromGroupHandler struct{}

func NewDeleteSharedProjectFromGroupHandler() *DeleteSharedProjectFromGroupHandler {
	return &DeleteSharedProjectFromGroupHandler{}
}

type DeleteSharedProjectFromGroupInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	GroupId   int          `json:"group_id,omitempty"`
}

type DeleteSharedProjectFromGroupOutputs struct{}

func (h *DeleteSharedProjectFromGroupHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteSharedProjectFromGroupInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Projects.DeleteSharedProjectFromGroup(inputs.ProjectId.String(), inputs.GroupId)
	if err != nil {
		return nil, err
	}
	return &DeleteSharedProjectFromGroupOutputs{}, nil
}
