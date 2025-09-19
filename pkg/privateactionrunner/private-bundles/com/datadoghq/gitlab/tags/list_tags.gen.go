package com_datadoghq_gitlab_tags

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListTagsHandler struct{}

func NewListTagsHandler() *ListTagsHandler {
	return &ListTagsHandler{}
}

type ListTagsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListTagsOptions
}

type ListTagsOutputs struct {
	Tags []*gitlab.Tag `json:"tags"`
}

func (h *ListTagsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListTagsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	tags, _, err := git.Tags.ListTags(inputs.ProjectId.String(), inputs.ListTagsOptions)
	if err != nil {
		return nil, err
	}
	return &ListTagsOutputs{Tags: tags}, nil
}
