package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ShareProjectWithGroupHandler struct{}

func NewShareProjectWithGroupHandler() *ShareProjectWithGroupHandler {
	return &ShareProjectWithGroupHandler{}
}

type ShareProjectWithGroupInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ShareWithGroupOptions
}

type ShareProjectWithGroupOutputs struct{}

func (h *ShareProjectWithGroupHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ShareProjectWithGroupInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Projects.ShareProjectWithGroup(inputs.ProjectId.String(), inputs.ShareWithGroupOptions)
	if err != nil {
		return nil, err
	}
	return &ShareProjectWithGroupOutputs{}, nil
}
