// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repositories

import (
	"context"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ContributorsHandler struct{}

func NewContributorsHandler() *ContributorsHandler {
	return &ContributorsHandler{}
}

type ContributorsInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*ListContributorsOptions
}

type ListContributorsOptions struct {
	Ref *string `url:"ref, omitempty" json:"ref,omitempty"`
	*gitlab.ListContributorsOptions
}

type ContributorsOutputs struct {
	Contributors []*gitlab.Contributor `json:"contributors"`
}

func (h *ContributorsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ContributorsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("projects/%s/repository/contributors", gitlab.PathEscape(inputs.ProjectId.String()))

	req, err := git.NewRequest(http.MethodGet, u, inputs.ListContributorsOptions, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab http request: %w", err)
	}
	var contributors []*gitlab.Contributor
	_, err = git.Do(req, &contributors)
	if err != nil {
		return nil, err
	}

	return &ContributorsOutputs{Contributors: contributors}, nil
}
