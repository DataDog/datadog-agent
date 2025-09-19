package com_datadoghq_gitlab_environments

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListEnvironmentsHandler struct{}

func NewListEnvironmentsHandler() *ListEnvironmentsHandler {
	return &ListEnvironmentsHandler{}
}

type ListEnvironmentsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListEnvironmentsOptions
}

type ListEnvironmentsOutputs struct {
	Environments []*gitlab.Environment `json:"environments"`
}

func (h *ListEnvironmentsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListEnvironmentsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	environments, _, err := git.Environments.ListEnvironments(inputs.ProjectId.String(), inputs.ListEnvironmentsOptions)
	if err != nil {
		return nil, err
	}
	return &ListEnvironmentsOutputs{Environments: environments}, nil
}
