package com_datadoghq_gitlab_repositories

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GenerateChangelogDataHandler struct{}

func NewGenerateChangelogDataHandler() *GenerateChangelogDataHandler {
	return &GenerateChangelogDataHandler{}
}

type GenerateChangelogDataInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	gitlab.GenerateChangelogDataOptions
}

type GenerateChangelogDataOutputs struct {
	ChangelogData *gitlab.ChangelogData `json:"changelog_data"`
}

func (h *GenerateChangelogDataHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GenerateChangelogDataInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	changelogData, _, err := git.Repositories.GenerateChangelogData(inputs.ProjectId.String(), inputs.GenerateChangelogDataOptions)
	if err != nil {
		return nil, err
	}
	return &GenerateChangelogDataOutputs{ChangelogData: changelogData}, nil
}
