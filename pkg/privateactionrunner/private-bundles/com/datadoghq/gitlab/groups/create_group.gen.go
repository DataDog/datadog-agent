package com_datadoghq_gitlab_groups

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateGroupHandler struct{}

func NewCreateGroupHandler() *CreateGroupHandler {
	return &CreateGroupHandler{}
}

type CreateGroupInputs struct {
	*gitlab.CreateGroupOptions
}

type CreateGroupOutputs struct {
	Group *gitlab.Group `json:"group"`
}

func (h *CreateGroupHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateGroupInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	group, _, err := git.Groups.CreateGroup(inputs.CreateGroupOptions)
	if err != nil {
		return nil, err
	}
	return &CreateGroupOutputs{Group: group}, nil
}
