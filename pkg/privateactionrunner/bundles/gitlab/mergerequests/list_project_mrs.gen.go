// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_merge_requests

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type ListProjectMergeRequestsHandler struct{}

func NewListProjectMergeRequestsHandler() *ListProjectMergeRequestsHandler {
	return &ListProjectMergeRequestsHandler{}
}

type ListProjectMergeRequestsInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListProjectMergeRequestsOptions
}

type ListProjectMergeRequestsOutputs struct {
	BasicMergeRequests []*gitlab.BasicMergeRequest `json:"merge_requests"`
}

func (h *ListProjectMergeRequestsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListProjectMergeRequestsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	basicMergeRequests, _, err := git.MergeRequests.ListProjectMergeRequests(inputs.ProjectId.String(), inputs.ListProjectMergeRequestsOptions)
	if err != nil {
		return nil, err
	}
	return &ListProjectMergeRequestsOutputs{BasicMergeRequests: basicMergeRequests}, nil
}
