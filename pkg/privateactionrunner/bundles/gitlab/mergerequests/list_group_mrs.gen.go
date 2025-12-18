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

type ListGroupMergeRequestsHandler struct{}

func NewListGroupMergeRequestsHandler() *ListGroupMergeRequestsHandler {
	return &ListGroupMergeRequestsHandler{}
}

type ListGroupMergeRequestsInputs struct {
	GroupId support.GitlabID `json:"group_id,omitempty"`
	*gitlab.ListGroupMergeRequestsOptions
}

type ListGroupMergeRequestsOutputs struct {
	BasicMergeRequests []*gitlab.BasicMergeRequest `json:"merge_requests"`
}

func (h *ListGroupMergeRequestsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListGroupMergeRequestsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	basicMergeRequests, _, err := git.MergeRequests.ListGroupMergeRequests(inputs.GroupId.String(), inputs.ListGroupMergeRequestsOptions)
	if err != nil {
		return nil, err
	}
	return &ListGroupMergeRequestsOutputs{BasicMergeRequests: basicMergeRequests}, nil
}
