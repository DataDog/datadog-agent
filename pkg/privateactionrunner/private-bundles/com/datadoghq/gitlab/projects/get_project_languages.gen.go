package com_datadoghq_gitlab_projects

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetProjectLanguagesHandler struct{}

func NewGetProjectLanguagesHandler() *GetProjectLanguagesHandler {
	return &GetProjectLanguagesHandler{}
}

type GetProjectLanguagesInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
}

type GetProjectLanguagesOutputs struct {
	ProjectLanguages *gitlab.ProjectLanguages `json:"project_languages"`
}

func (h *GetProjectLanguagesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetProjectLanguagesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	projectLanguages, _, err := git.Projects.GetProjectLanguages(inputs.ProjectId.String())
	if err != nil {
		return nil, err
	}
	return &GetProjectLanguagesOutputs{ProjectLanguages: projectLanguages}, nil
}
