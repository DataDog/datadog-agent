package com_datadoghq_gitlab_tags

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateTagHandler struct{}

func NewCreateTagHandler() *CreateTagHandler {
	return &CreateTagHandler{}
}

type CreateTagInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.CreateTagOptions
}

type CreateTagOutputs struct {
	Tag *gitlab.Tag `json:"tag"`
}

func (h *CreateTagHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateTagInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	tag, _, err := git.Tags.CreateTag(inputs.ProjectId.String(), inputs.CreateTagOptions)
	if err != nil {
		return nil, err
	}
	return &CreateTagOutputs{Tag: tag}, nil
}
