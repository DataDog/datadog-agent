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

type AcceptMergeRequestHandler struct{}

func NewAcceptMergeRequestHandler() *AcceptMergeRequestHandler {
	return &AcceptMergeRequestHandler{}
}

type AcceptMergeRequestInputs struct {
	ProjectId       support.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int64            `json:"merge_request_iid,omitempty"`
	*gitlab.AcceptMergeRequestOptions
}

type AcceptMergeRequestOutputs struct {
	MergeRequest *gitlab.MergeRequest `json:"merge_request"`
}

func (h *AcceptMergeRequestHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[AcceptMergeRequestInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	mergeRequest, _, err := git.MergeRequests.AcceptMergeRequest(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.AcceptMergeRequestOptions)
	if err != nil {
		return nil, err
	}
	return &AcceptMergeRequestOutputs{MergeRequest: mergeRequest}, nil
}
