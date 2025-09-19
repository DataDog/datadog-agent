package com_datadoghq_gitlab_repositories

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type AddChangelogHandler struct{}

func NewAddChangelogHandler() *AddChangelogHandler {
	return &AddChangelogHandler{}
}

type AddChangelogInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.AddChangelogOptions
}

type AddChangelogOutputs struct{}

func (h *AddChangelogHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[AddChangelogInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Repositories.AddChangelog(inputs.ProjectId.String(), inputs.AddChangelogOptions)
	if err != nil {
		return nil, err
	}
	return &AddChangelogOutputs{}, nil
}
