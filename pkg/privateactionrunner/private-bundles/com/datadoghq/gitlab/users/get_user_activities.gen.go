package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetUserActivitiesHandler struct{}

func NewGetUserActivitiesHandler() *GetUserActivitiesHandler {
	return &GetUserActivitiesHandler{}
}

type GetUserActivitiesInputs struct {
	*gitlab.GetUserActivitiesOptions
}

type GetUserActivitiesOutputs struct {
	UserActivities []*gitlab.UserActivity `json:"user_activities"`
}

func (h *GetUserActivitiesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetUserActivitiesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	userActivities, _, err := git.Users.GetUserActivities(inputs.GetUserActivitiesOptions)
	if err != nil {
		return nil, err
	}
	return &GetUserActivitiesOutputs{UserActivities: userActivities}, nil
}
