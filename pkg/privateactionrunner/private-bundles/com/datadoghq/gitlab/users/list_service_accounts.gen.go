package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListServiceAccountsHandler struct{}

func NewListServiceAccountsHandler() *ListServiceAccountsHandler {
	return &ListServiceAccountsHandler{}
}

type ListServiceAccountsInputs struct {
	*gitlab.ListServiceAccountsOptions
}

type ListServiceAccountsOutputs struct {
	ServiceAccounts []*gitlab.ServiceAccount `json:"service_accounts"`
}

func (h *ListServiceAccountsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListServiceAccountsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	serviceAccounts, _, err := git.Users.ListServiceAccounts(inputs.ListServiceAccountsOptions)
	if err != nil {
		return nil, err
	}
	return &ListServiceAccountsOutputs{ServiceAccounts: serviceAccounts}, nil
}
