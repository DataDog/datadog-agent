package com_datadoghq_gitlab_groups

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListGroupsHandler struct{}

func NewListGroupsHandler() *ListGroupsHandler {
	return &ListGroupsHandler{}
}

type ListGroupsInputs struct {
	*gitlab.ListGroupsOptions
}

type ListGroupsOutputs struct {
	Groups []*gitlab.Group `json:"groups"`
}

func (h *ListGroupsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListGroupsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	groups, _, err := git.Groups.ListGroups(inputs.ListGroupsOptions)
	if err != nil {
		return nil, err
	}
	return &ListGroupsOutputs{Groups: groups}, nil
}
