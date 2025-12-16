// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type CreateServiceAccountUserHandler struct{}

func NewCreateServiceAccountUserHandler() *CreateServiceAccountUserHandler {
	return &CreateServiceAccountUserHandler{}
}

type CreateServiceAccountUserInputs struct {
	*gitlab.CreateServiceAccountUserOptions
}

type ServiceAccount struct {
	*gitlab.ServiceAccount
	Email string `json:"email"`
}

type CreateServiceAccountUserOutputs struct {
	ServiceAccount *ServiceAccount `json:"service_account"`
}

func (h *CreateServiceAccountUserHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[CreateServiceAccountUserInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	user, _, err := git.Users.CreateServiceAccountUser(inputs.CreateServiceAccountUserOptions)
	if err != nil {
		return nil, err
	}
	// The api returns a ServiceAccount but the library returns a User (subtype of ServiceAccount)
	serviceAccount := &ServiceAccount{
		ServiceAccount: &gitlab.ServiceAccount{
			ID:       user.ID,
			Name:     user.Name,
			Username: user.Username,
		},
		Email: user.Email,
	}
	return &CreateServiceAccountUserOutputs{ServiceAccount: serviceAccount}, nil
}
