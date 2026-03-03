// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_merge_requests

import (
	"context"
	"encoding/json"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ApproveMergeRequestHandler struct{}

func NewApproveMergeRequestHandler() *ApproveMergeRequestHandler {
	return &ApproveMergeRequestHandler{}
}

type ApproveMergeRequestInputs struct {
	ProjectId       json.Number `json:"project_id,omitempty"`
	MergeRequestIid int64       `json:"merge_request_iid,omitempty"`
	*gitlab.ApproveMergeRequestOptions
}

type ApproveMergeRequestOutputs struct {
	MergeRequestApprovals *gitlab.MergeRequestApprovals `json:"merge_request_approvals"`
}

func (h *ApproveMergeRequestHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ApproveMergeRequestInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	mergeRequestApprovals, _, err := git.MergeRequestApprovals.ApproveMergeRequest(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.ApproveMergeRequestOptions)
	if err != nil {
		return nil, err
	}
	return &ApproveMergeRequestOutputs{MergeRequestApprovals: mergeRequestApprovals}, nil
}
