package com_datadoghq_gitlab_groups

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetGroupHandler struct{}

func NewGetGroupHandler() *GetGroupHandler {
	return &GetGroupHandler{}
}

type GetGroupInputs struct {
	GroupId lib.GitlabID `json:"group_id,omitempty"`
	*gitlab.GetGroupOptions
}

type GetGroupOutputs struct {
	Group *gitlab.Group `json:"group"`
}

func (h *GetGroupHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetGroupInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	group, _, err := git.Groups.GetGroup(inputs.GroupId.String(), inputs.GetGroupOptions)
	if err != nil {
		return nil, err
	}
	return &GetGroupOutputs{Group: group}, nil
}
